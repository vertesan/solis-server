package viber

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
)

// photoPanoramaCatalogFiles retains only the embedded file index. The raw
// YAML bytes read by LoadPhotoPanoramaCatalog are local and can be reclaimed
// after decoding.
//
//go:embed catalogdata/photo_panorama.yaml
var photoPanoramaCatalogFiles embed.FS

type PhotoPanoramaCatalog struct {
	Musics []PhotoPanoramaCatalogEntry `yaml:"musics"`
}

type PhotoPanoramaCatalogEntry struct {
	MusicID           string `yaml:"musicId"`
	StageID           string `yaml:"stageId"`
	Order             int32  `yaml:"order"`
	UnlockConditionID string `yaml:"unlockConditionId"`
	ViewConditionID   string `yaml:"viewConditionId"`
}

func LoadPhotoPanoramaCatalog() (*PhotoPanoramaCatalog, error) {
	data, err := photoPanoramaCatalogFiles.ReadFile("catalogdata/photo_panorama.yaml")
	if err != nil {
		return nil, fmt.Errorf("read photo panorama catalog: %w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var catalog PhotoPanoramaCatalog
	if err := decoder.Decode(&catalog); err != nil {
		return nil, fmt.Errorf("decode photo panorama catalog: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode photo panorama catalog: multiple YAML documents are not supported")
		}
		return nil, fmt.Errorf("decode photo panorama catalog: %w", err)
	}

	if len(catalog.Musics) == 0 {
		return nil, fmt.Errorf("photo panorama catalog is empty")
	}
	musicIDs := make(map[string]struct{}, len(catalog.Musics))
	orders := make(map[int32]struct{}, len(catalog.Musics))
	for index, entry := range catalog.Musics {
		if entry.MusicID == "" || entry.StageID == "" || entry.UnlockConditionID == "" || entry.ViewConditionID == "" {
			return nil, fmt.Errorf("photo panorama catalog entry %d has an empty required field", index)
		}
		if entry.MusicID != strings.TrimSpace(entry.MusicID) ||
			entry.StageID != strings.TrimSpace(entry.StageID) ||
			entry.UnlockConditionID != strings.TrimSpace(entry.UnlockConditionID) ||
			entry.ViewConditionID != strings.TrimSpace(entry.ViewConditionID) {
			return nil, fmt.Errorf("photo panorama catalog entry %d has surrounding whitespace", index)
		}
		if entry.Order <= 0 {
			return nil, fmt.Errorf("photo panorama catalog entry %q has a non-positive order", entry.MusicID)
		}
		if _, duplicate := musicIDs[entry.MusicID]; duplicate {
			return nil, fmt.Errorf("photo panorama catalog has duplicate musicId %q", entry.MusicID)
		}
		if _, duplicate := orders[entry.Order]; duplicate {
			return nil, fmt.Errorf("photo panorama catalog has duplicate order %d", entry.Order)
		}
		musicIDs[entry.MusicID] = struct{}{}
		orders[entry.Order] = struct{}{}
	}
	return &catalog, nil
}
