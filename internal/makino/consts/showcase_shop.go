package consts

import "github.com/vertesan/solis-server/internal/proto/papi"

const (
	ShowcaseGoodsCatalogShopID = "shop_top_shop-showcase-goods-catalog"
	ShowcaseFrameShopID        = "shop_top_shop-showcase-frame"
	ShowcaseMusicShopID        = "shop_top_shop-showcase-music"
)

func GetUserInitialNotiShowcaseShopItem() *papi.NotiShopItem {
	return &papi.NotiShopItem{ShopId: ShowcaseGoodsCatalogShopID, ShopItemIds: []string{}}
}

func GetUserInitialShowcaseToyShopIds() []string {
	return []string{
		ShowcaseGoodsCatalogShopID,
		ShowcaseFrameShopID,
		ShowcaseMusicShopID,
	}
}
