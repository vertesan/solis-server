package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type catalogInput struct {
	notices *smodel.NoticeCatalog
	shops   []*smodel.ShopCatalogEntry
	booths  []*papi.ExchangeBooth
}

func (c *catalogInput) count() int {
	n := len(c.notices.Notices) + len(c.notices.MalfunctionNotices) + len(c.notices.PrNotices) + len(c.shops) + len(c.booths)
	if c.notices.ShowcaseShopNotice != nil {
		n++
	}
	return n
}

func decodeDocuments(kind string, documents [][]byte) (*catalogInput, error) {
	input := &catalogInput{notices: &smodel.NoticeCatalog{}}
	for file, data := range documents {
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		for document := 1; ; document++ {
			var value map[string]any
			err := decoder.Decode(&value)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("input %d document %d: %w", file+1, document, err)
			}
			if len(value) == 0 {
				return nil, fmt.Errorf("input %d document %d is empty", file+1, document)
			}
			encoded, err := json.Marshal(value)
			if err == nil {
				err = input.decode(kind, encoded, value)
			}
			if err != nil {
				return nil, fmt.Errorf("input %d document %d: %w", file+1, document, err)
			}
		}
	}
	ids := make(map[string]bool)
	checkID := func(id string) error {
		if strings.TrimSpace(id) == "" {
			return errors.New("catalog entry requires a nonempty id")
		}
		if ids[id] {
			return fmt.Errorf("duplicate catalog id %q", id)
		}
		ids[id] = true
		return nil
	}
	for _, group := range [][]*papi.NoticeInfo{input.notices.Notices, input.notices.MalfunctionNotices, input.notices.PrNotices} {
		for _, entry := range group {
			if err := checkID(entry.GetId()); err != nil {
				return nil, err
			}
		}
	}
	if notice := input.notices.ShowcaseShopNotice; notice != nil && strings.TrimSpace(notice.GetId()) == "" {
		return nil, errors.New("showcaseShopNotice requires a nonempty id")
	}
	for _, entry := range input.shops {
		if entry == nil {
			return nil, errors.New("shop entry cannot be null")
		}
		if err := checkID(entry.ShopId); err != nil {
			return nil, err
		}
	}
	for _, entry := range input.booths {
		if err := checkID(entry.GetId()); err != nil {
			return nil, err
		}
	}
	return input, nil
}

func (c *catalogInput) decode(kind string, data []byte, keys map[string]any) error {
	switch kind {
	case "notice":
		if _, single := keys["id"]; single {
			notice := new(papi.NoticeInfo)
			if err := unmarshalProto(data, notice); err != nil {
				return err
			}
			c.notices.Notices = append(c.notices.Notices, notice)
			return nil
		}
		var envelope struct {
			Notices            []json.RawMessage `json:"notices"`
			MalfunctionNotices []json.RawMessage `json:"malfunctionNotices"`
			PrNotices          []json.RawMessage `json:"prNotices"`
			ShowcaseShopNotice json.RawMessage   `json:"showcaseShopNotice"`
		}
		if err := strictJSON(data, &envelope); err != nil {
			return err
		}
		for i, group := range [][]json.RawMessage{envelope.Notices, envelope.MalfunctionNotices, envelope.PrNotices} {
			output := []*[]*papi.NoticeInfo{&c.notices.Notices, &c.notices.MalfunctionNotices, &c.notices.PrNotices}[i]
			for _, raw := range group {
				notice := new(papi.NoticeInfo)
				if err := unmarshalProto(raw, notice); err != nil {
					return err
				}
				*output = append(*output, notice)
			}
		}
		if !isNull(envelope.ShowcaseShopNotice) {
			if c.notices.ShowcaseShopNotice != nil {
				return errors.New("duplicate showcaseShopNotice")
			}
			c.notices.ShowcaseShopNotice = new(papi.NoticeInfo)
			return unmarshalProto(envelope.ShowcaseShopNotice, c.notices.ShowcaseShopNotice)
		}
	case "shop":
		entries := []json.RawMessage{data}
		if _, envelope := keys["shops"]; envelope {
			var wrapper struct {
				Shops []json.RawMessage `json:"shops"`
			}
			if err := strictJSON(data, &wrapper); err != nil {
				return err
			}
			entries = wrapper.Shops
		}
		for _, raw := range entries {
			entry, err := decodeShop(raw)
			if err != nil {
				return err
			}
			c.shops = append(c.shops, entry)
		}
	case "exchange":
		entries := []json.RawMessage{data}
		if _, envelope := keys["booths"]; envelope {
			var wrapper struct {
				Booths []json.RawMessage `json:"booths"`
			}
			if err := strictJSON(data, &wrapper); err != nil {
				return err
			}
			entries = wrapper.Booths
		}
		for _, raw := range entries {
			booth := new(papi.ExchangeBooth)
			if err := unmarshalProto(raw, booth); err != nil {
				return err
			}
			c.booths = append(c.booths, booth)
		}
	}
	return nil
}

func decodeShop(data []byte) (*smodel.ShopCatalogEntry, error) {
	var raw struct {
		ShopID                string            `json:"shopId"`
		Info                  json.RawMessage   `json:"info"`
		Items                 []json.RawMessage `json:"items"`
		LoginBonusPackageItem json.RawMessage   `json:"loginBonusPackageItem"`
		OriginalProductIDs    map[string]string `json:"originalProductIds"`
	}
	if err := strictJSON(data, &raw); err != nil {
		return nil, err
	}
	entry := &smodel.ShopCatalogEntry{ShopId: raw.ShopID, Info: new(papi.ShopInfo), OriginalProductIds: raw.OriginalProductIDs}
	if err := unmarshalProto(raw.Info, entry.Info); err != nil {
		return nil, fmt.Errorf("shop info: %w", err)
	}
	for _, rawItem := range raw.Items {
		item := new(papi.ShopItem)
		if err := unmarshalProto(rawItem, item); err != nil {
			return nil, err
		}
		entry.Items = append(entry.Items, item)
	}
	if !isNull(raw.LoginBonusPackageItem) {
		entry.LoginBonusPackageItem = new(papi.LoginBonusPackageItem)
		if err := unmarshalProto(raw.LoginBonusPackageItem, entry.LoginBonusPackageItem); err != nil {
			return nil, err
		}
	}
	return entry, nil
}

func strictJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func isNull(data []byte) bool {
	return len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null"))
}

func unmarshalProto(data []byte, value proto.Message) error {
	if isNull(data) {
		return errors.New("catalog entry cannot be null")
	}
	return protojson.Unmarshal(data, value)
}

func marshalYAML(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	// YAML decoding retains integer precision, including int64 timestamps; an
	// intermediate encoding/json map would round large integers via float64.
	var document any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	return yaml.Marshal(document)
}
