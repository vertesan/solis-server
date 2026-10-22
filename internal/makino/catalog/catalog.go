// Package catalog manages the server's editable catalogs. YAML is an import and
// export format only; running servers read the committed database definitions.
package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Apply validates every input before modifying any catalog. Add replaces entries
// with matching IDs; overwrite replaces the whole catalog. Clear requires no
// documents so an accidentally empty directory cannot silently delete content.
func Apply(ctx context.Context, kind, mode string, documents [][]byte) error {
	if kind != "notice" && kind != "shop" && kind != "exchange" {
		return fmt.Errorf("unknown catalog %q", kind)
	}
	if mode != "add" && mode != "overwrite" && mode != "clear" {
		return fmt.Errorf("unknown catalog operation %q", mode)
	}
	if mode == "clear" && len(documents) != 0 {
		return errors.New("clear does not accept input files")
	}
	input, err := decodeDocuments(kind, documents)
	if err != nil {
		return err
	}
	if mode != "clear" && input.count() == 0 {
		return errors.New("catalog input is empty; use clear to remove all entries")
	}
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		switch kind {
		case "notice":
			notices := input.notices
			if mode == "add" {
				current, err := opsolis.FindNoticeCatalog(ctx)
				if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
					return nil, err
				}
				if current != nil {
					notices = mergeNotices(current, input.notices)
				}
			}
			return nil, opsolis.UpsertNoticeCatalog(ctx, notices)
		case "shop":
			shops := input.shops
			if mode == "add" {
				current, err := opsolis.FindAllShopCatalogEntries(ctx)
				if err != nil {
					return nil, err
				}
				shops = mergeEntries(current, shops, func(s *smodel.ShopCatalogEntry) string { return s.ShopId })
			}
			if err := validateShops(ctx, shops); err != nil {
				return nil, err
			}
			if mode != "add" {
				if err := opsolis.DeleteAllShopCatalogEntries(ctx); err != nil {
					return nil, err
				}
			}
			for _, shop := range input.shops {
				if err := opsolis.UpsertShopCatalogEntry(ctx, shop); err != nil {
					return nil, err
				}
			}
		case "exchange":
			booths := input.booths
			if mode == "add" {
				current, err := opsolis.FindAllExchangeBooths(ctx)
				if err != nil {
					return nil, err
				}
				booths = mergeEntries(current, booths, func(b *papi.ExchangeBooth) string { return b.GetId() })
			}
			if err := validateBooths(ctx, booths); err != nil {
				return nil, err
			}
			if mode != "add" {
				if err := opsolis.DeleteAllExchangeBooths(ctx); err != nil {
					return nil, err
				}
			}
			for _, booth := range input.booths {
				if err := opsolis.UpsertExchangeBooth(ctx, booth); err != nil {
					return nil, err
				}
			}
		}
		return nil, nil
	})
}

// Export returns a complete editable snapshot, including the private product
// mappings needed to grant converted real-money bundles their original diamonds.
func Export(ctx context.Context, kind string) ([]byte, error) {
	var value any
	switch kind {
	case "notice":
		catalog, err := opsolis.FindNoticeCatalog(ctx)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		if catalog == nil {
			catalog = &smodel.NoticeCatalog{}
		}
		value = catalog
	case "shop":
		shops, err := opsolis.FindAllShopCatalogEntries(ctx)
		if err != nil {
			return nil, err
		}
		value = struct {
			Shops []*smodel.ShopCatalogEntry `json:"shops"`
		}{shops}
	case "exchange":
		booths, err := opsolis.FindAllExchangeBooths(ctx)
		if err != nil {
			return nil, err
		}
		value = struct {
			Booths []*papi.ExchangeBooth `json:"booths"`
		}{booths}
	default:
		return nil, fmt.Errorf("unknown catalog %q", kind)
	}
	return marshalYAML(value)
}

func mergeEntries[T any](existing, incoming []T, id func(T) string) []T {
	byID := make(map[string]int, len(existing)+len(incoming))
	result := append([]T(nil), existing...)
	for i, entry := range result {
		byID[id(entry)] = i
	}
	for _, entry := range incoming {
		if i, ok := byID[id(entry)]; ok {
			result[i] = entry
		} else {
			byID[id(entry)] = len(result)
			result = append(result, entry)
		}
	}
	return result
}

func mergeNotices(current, incoming *smodel.NoticeCatalog) *smodel.NoticeCatalog {
	// An ID belongs to exactly one category; an upsert can move it between tabs.
	replaced := make(map[string]bool)
	for _, group := range [][]*papi.NoticeInfo{incoming.Notices, incoming.MalfunctionNotices, incoming.PrNotices} {
		for _, notice := range group {
			replaced[notice.Id] = true
		}
	}
	keep := func(notices []*papi.NoticeInfo) []*papi.NoticeInfo {
		result := make([]*papi.NoticeInfo, 0, len(notices))
		for _, notice := range notices {
			if !replaced[notice.GetId()] {
				result = append(result, notice)
			}
		}
		return result
	}
	showcaseNotice := incoming.ShowcaseShopNotice
	if showcaseNotice == nil {
		showcaseNotice = current.ShowcaseShopNotice
	}
	return &smodel.NoticeCatalog{
		Notices:            append(keep(current.Notices), incoming.Notices...),
		MalfunctionNotices: append(keep(current.MalfunctionNotices), incoming.MalfunctionNotices...),
		PrNotices:          append(keep(current.PrNotices), incoming.PrNotices...),
		ShowcaseShopNotice: showcaseNotice,
	}
}
