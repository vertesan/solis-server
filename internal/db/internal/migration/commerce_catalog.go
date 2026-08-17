package migration

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/proto/papi"
)

// The commerce catalog is server-controlled content rather than master data.
// Keep the captured payloads intact so fields are not silently lost while
// translating large catalogs into handwritten Go literals.

// UpsertCommerceCatalogs installs the captured Exchange and
// Shop catalogs. Per-user unlock, purchase, and stock state stays in user DB.
func UpsertCommerceCatalogs(ctx context.Context) error {
	booths, err := commerceExchangeBooths()
	if err != nil {
		return err
	}
	if err := opsolis.DeleteAllExchangeBooths(ctx); err != nil {
		return err
	}
	for _, booth := range booths {
		if err := opsolis.UpsertExchangeBooth(ctx, booth); err != nil {
			return err
		}
	}

	shops, err := commerceShopEntries()
	if err != nil {
		return err
	}
	if err := opsolis.DeleteAllShopCatalogEntries(ctx); err != nil {
		return err
	}
	for _, entry := range shops {
		if err := opsolis.UpsertShopCatalogEntry(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

func commerceExchangeBooths() ([]*papi.ExchangeBooth, error) {
	response := new(papi.ExchangeListResponse)
	if err := unmarshalMigrationProtoYAML("catalogdata/exchange.yaml", response); err != nil {
		return nil, err
	}
	return response.Booths, nil
}

func commerceShopEntries() ([]*smodel.ShopCatalogEntry, error) {
	response := new(papi.ShopListResponse)
	if err := unmarshalMigrationProtoYAML("catalogdata/shops.yaml", response); err != nil {
		return nil, err
	}

	entries := make([]*smodel.ShopCatalogEntry, 0, len(response.Shops))
	byId := make(map[string]*smodel.ShopCatalogEntry, len(response.Shops))
	for _, info := range response.Shops {
		entry := &smodel.ShopCatalogEntry{
			ShopId: info.Id,
			Info:   info,
			Items:  []*papi.ShopItem{},
		}
		entries = append(entries, entry)
		byId[entry.ShopId] = entry
	}

	if err := assignCommerceShopItems(byId, "shop_top_special-shop-pack2-26-0731", "catalogdata/special_items.yaml"); err != nil {
		return nil, err
	}
	if err := assignCommerceShopItems(byId, "shop_top_dress-shop-pack-26-0731", "catalogdata/dress_items.yaml"); err != nil {
		return nil, err
	}
	if err := assignCommerceLoginPackage(byId, "shop_login_login-pack-01", "catalogdata/login_pack_01.yaml"); err != nil {
		return nil, err
	}
	if err := assignCommerceLoginPackage(byId, "shop_login_login-pack-02", "catalogdata/login_pack_02.yaml"); err != nil {
		return nil, err
	}
	return entries, nil
}

func assignCommerceShopItems(entries map[string]*smodel.ShopCatalogEntry, shopId, path string) error {
	entry, ok := entries[shopId]
	if !ok {
		return fmt.Errorf("commerce shop %q is missing from list snapshot", shopId)
	}
	response := new(papi.ShopListItemResponse)
	if err := unmarshalMigrationProtoYAML(path, response); err != nil {
		return err
	}
	entry.Items = response.ShopItems
	return nil
}

func assignCommerceLoginPackage(entries map[string]*smodel.ShopCatalogEntry, shopId, path string) error {
	entry, ok := entries[shopId]
	if !ok {
		return fmt.Errorf("commerce shop %q is missing from list snapshot", shopId)
	}
	response := new(papi.ShopGetLoginBonusPackageItemResponse)
	if err := unmarshalMigrationProtoYAML(path, response); err != nil {
		return err
	}
	entry.LoginBonusPackageItem = response.LoginBonusPackageItem
	return nil
}
