package viber

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

const (
	ShowcaseGoodsShopID   = "shop_top_shop-showcase-goods-catalog"
	ShowcaseFrameShopID   = "shop_top_shop-showcase-frame"
	ShowcaseMusicShopID   = "shop_top_shop-showcase-music"
	ShowcaseMedalItemID   = "item-showcase_medal"
	showcaseGoodsItemCost = int32(100)
)

const (
	showcaseColorCode           = "71BDFD"
	showcaseBackgroundColorCode = "3388D1"
	showcaseItemIDPrefix        = "shop_showcase_catalog_"
	showcaseMusicItemIDPrefix   = ShowcaseMusicShopID + "_"
	showcaseFrameItemIDPrefix   = "item-showcase_"
)

type ShowcaseCommerceCatalog struct {
	shops     []*smodel.ShopCatalogEntry
	shopsByID map[string]*smodel.ShopCatalogEntry
	itemsByID map[string]showcaseCommerceItem
}

type showcaseCommerceItem struct {
	shop *smodel.ShopCatalogEntry
	item *papi.ShopItem
}

func BuildShowcaseCommerceCatalog(ctx context.Context, suid string) (*ShowcaseCommerceCatalog, error) {
	toys, err := opmaster.FindAllShowcaseToys(ctx)
	if err != nil {
		return nil, err
	}
	frames, err := opmaster.FindAllShowcaseFrames(ctx)
	if err != nil {
		return nil, err
	}
	showcaseMusics, err := opmaster.FindAllShowcaseMusics(ctx)
	if err != nil {
		return nil, err
	}
	musics, err := opmaster.FindAllMusics(ctx)
	if err != nil {
		return nil, err
	}
	userItems, err := opuser.FindAllUserItems(ctx, suid)
	if err != nil {
		return nil, err
	}
	userShowcaseMusics, err := opuser.FindAllUserShowcaseMusics(ctx, suid)
	if err != nil {
		return nil, err
	}
	userShowcaseToys, err := opuser.FindAllUserShowcaseToys(ctx, suid)
	if err != nil {
		return nil, err
	}

	ownedItemIDs := make(map[string]struct{})
	now := time.Now().UnixMilli()
	for _, item := range userItems {
		if item.GetAmount() > 0 && (item.GetExpiredTime() == 0 || item.GetExpiredTime() > now) {
			ownedItemIDs[item.GetItemId()] = struct{}{}
		}
	}
	ownedMusicIDs := make(map[string]struct{}, len(userShowcaseMusics))
	for _, music := range userShowcaseMusics {
		ownedMusicIDs[music.GetShowcaseMusicId()] = struct{}{}
	}
	ownedToyIDs := make(map[string]struct{}, len(userShowcaseToys))
	for _, toy := range userShowcaseToys {
		if toy.GetAmount() > 0 {
			ownedToyIDs[toy.GetShowcaseToyId()] = struct{}{}
		}
	}
	musicNames := make(map[string]string, len(musics))
	for _, music := range musics {
		musicNames[music.GetId()] = music.GetName()
	}

	catalog := &ShowcaseCommerceCatalog{
		shopsByID: make(map[string]*smodel.ShopCatalogEntry, 3),
		itemsByID: make(map[string]showcaseCommerceItem, len(toys)+len(frames)+len(showcaseMusics)),
	}
	catalog.addShop(buildShowcaseGoodsShop(toys, ownedToyIDs))
	catalog.addShop(buildShowcaseFrameShop(frames, ownedItemIDs))
	musicShop, err := buildShowcaseMusicShop(showcaseMusics, musicNames, ownedMusicIDs)
	if err != nil {
		return nil, err
	}
	catalog.addShop(musicShop)
	return catalog, nil
}

func ShowcaseCommerceShopIDs() []string {
	return []string{ShowcaseGoodsShopID, ShowcaseMusicShopID, ShowcaseFrameShopID}
}

func IsShowcaseCommerceShopID(shopID string) bool {
	switch shopID {
	case ShowcaseGoodsShopID, ShowcaseFrameShopID, ShowcaseMusicShopID:
		return true
	default:
		return false
	}
}

func IsShowcaseCommerceItemID(itemID string) bool {
	return strings.HasPrefix(itemID, showcaseItemIDPrefix) || strings.HasPrefix(itemID, showcaseMusicItemIDPrefix)
}

func (c *ShowcaseCommerceCatalog) Shops() []*smodel.ShopCatalogEntry {
	if c == nil {
		return nil
	}
	return append([]*smodel.ShopCatalogEntry(nil), c.shops...)
}

func (c *ShowcaseCommerceCatalog) ShopIDs() []string {
	if c == nil {
		return nil
	}
	ids := make([]string, 0, len(c.shops))
	for _, shop := range c.shops {
		ids = append(ids, shop.ShopId)
	}
	return ids
}

func (c *ShowcaseCommerceCatalog) FindShop(shopID string) (*smodel.ShopCatalogEntry, bool) {
	if c == nil {
		return nil, false
	}
	shop, ok := c.shopsByID[shopID]
	return shop, ok
}

func (c *ShowcaseCommerceCatalog) FindItem(itemID string) (*smodel.ShopCatalogEntry, *papi.ShopItem, bool) {
	if c == nil {
		return nil, nil, false
	}
	entry, ok := c.itemsByID[itemID]
	return entry.shop, entry.item, ok
}

func (c *ShowcaseCommerceCatalog) addShop(shop *smodel.ShopCatalogEntry) {
	if shop == nil || shop.Info == nil || len(shop.Items) == 0 {
		return
	}
	c.shops = append(c.shops, shop)
	c.shopsByID[shop.ShopId] = shop
	for _, item := range shop.Items {
		if item != nil {
			c.itemsByID[item.GetId()] = showcaseCommerceItem{shop: shop, item: item}
		}
	}
}

func buildShowcaseGoodsShop(toys []*pmaster.ShowcaseToy, ownedToyIDs map[string]struct{}) *smodel.ShopCatalogEntry {
	sort.Slice(toys, func(i, j int) bool {
		if toys[i].GetOrder() != toys[j].GetOrder() {
			return toys[i].GetOrder() < toys[j].GetOrder()
		}
		return toys[i].GetId() < toys[j].GetId()
	})
	items := make([]*papi.ShopItem, 0, len(toys))
	for _, toy := range toys {
		if toy == nil || toy.GetId() == "" {
			continue
		}
		if toy.GetCategoryId() == "toy-category-figure" {
			if _, owned := ownedToyIDs[toy.GetId()]; owned {
				continue
			}
		}
		items = append(items, newShowcaseShopItem(
			showcaseItemIDPrefix+toy.GetId(),
			toy.GetName(),
			toy.GetDescription(),
			"rd_auto;showcase_toy;"+toy.GetId()+";1",
			showcaseGoodsItemCost,
			toy.GetOrder(),
		))
	}
	return newShowcaseShopCatalogEntry(
		ShowcaseGoodsShopID, "グッズカタログ", penum.ShopType_ShopType_DefaultShowcaseToyShop,
		101, "", items,
	)
}

func buildShowcaseFrameShop(frames []*pmaster.ShowcaseFrame, ownedItemIDs map[string]struct{}) *smodel.ShopCatalogEntry {
	sort.Slice(frames, func(i, j int) bool {
		if frames[i].GetOrder() != frames[j].GetOrder() {
			return frames[i].GetOrder() < frames[j].GetOrder()
		}
		return frames[i].GetId() < frames[j].GetId()
	})
	items := make([]*papi.ShopItem, 0, 3)
	for _, frame := range frames {
		cost, ok := showcaseFrameCost(frame.GetId())
		if !ok {
			continue
		}
		itemID := showcaseFrameItemIDPrefix + frame.GetId()
		if _, owned := ownedItemIDs[itemID]; owned {
			continue
		}
		shopItem := newShowcaseShopItem(
			showcaseItemIDPrefix+"toy-goods-"+frame.GetId(),
			"ケース："+frame.GetName(),
			"ケース："+frame.GetDescription(),
			"rd_auto;item;"+itemID+";1",
			cost,
			10_000+frame.GetOrder(),
		)
		shopItem.ThumbnailAssetId = "img_shop_item_thumb_toy-goods-" + frame.GetId()
		items = append(items, shopItem)
	}
	return newShowcaseShopCatalogEntry(
		ShowcaseFrameShopID, "グッズケースショップ", penum.ShopType_ShopType_OnlyShowcaseShop,
		104, "showcase-frame", items,
	)
}

func buildShowcaseMusicShop(
	showcaseMusics []*pmaster.ShowcaseMusic,
	musicNames map[string]string,
	ownedMusicIDs map[string]struct{},
) (*smodel.ShopCatalogEntry, error) {
	sort.Slice(showcaseMusics, func(i, j int) bool {
		if showcaseMusics[i].GetOrder() != showcaseMusics[j].GetOrder() {
			return showcaseMusics[i].GetOrder() < showcaseMusics[j].GetOrder()
		}
		return showcaseMusics[i].GetMusicId() < showcaseMusics[j].GetMusicId()
	})
	items := make([]*papi.ShopItem, 0, len(showcaseMusics))
	for _, showcaseMusic := range showcaseMusics {
		if showcaseMusic == nil || showcaseMusic.GetMusicId() == "" {
			continue
		}
		musicID := showcaseMusic.GetMusicId()
		if _, owned := ownedMusicIDs[musicID]; owned {
			continue
		}
		name, ok := musicNames[musicID]
		if !ok || name == "" {
			return nil, fmt.Errorf("showcase music %q is missing its Music master", musicID)
		}
		items = append(items, newShowcaseShopItem(
			showcaseMusicItemIDPrefix+musicID,
			name,
			name,
			"rd_auto;showcase_music;"+musicID+";1",
			showcaseGoodsItemCost,
			showcaseMusic.GetOrder(),
		))
	}
	return newShowcaseShopCatalogEntry(
		ShowcaseMusicShopID, "BGMショップ", penum.ShopType_ShopType_OnlyShowcaseShop,
		103, "showcase-music", items,
	), nil
}

func newShowcaseShopCatalogEntry(
	shopID string,
	name string,
	shopType penum.ShopType,
	order int32,
	thumbnailAssetID string,
	items []*papi.ShopItem,
) *smodel.ShopCatalogEntry {
	return &smodel.ShopCatalogEntry{
		ShopId: shopID,
		Info: &papi.ShopInfo{
			Id:                  shopID,
			Name:                name,
			Type:                shopType,
			Order:               order,
			ColorCode:           showcaseColorCode,
			BackgroundColorCode: showcaseBackgroundColorCode,
			ViewConditionId:     "cd-function_lock-showcase",
			ThumbnailAssetId:    thumbnailAssetID,
			ResetTimingType:     penum.ResetTimingType_ResetTimingType_Never,
		},
		Items: items,
	}
}

func newShowcaseShopItem(
	itemID string,
	name string,
	description string,
	rewardID string,
	cost int32,
	order int32,
) *papi.ShopItem {
	return &papi.ShopItem{
		Id:                  itemID,
		Name:                name,
		Description:         description,
		ConsumptionId:       fmt.Sprintf("con-item_showcase-medal-%d", cost),
		RewardId:            rewardID,
		Order:               order,
		ResetTimingType:     penum.ResetTimingType_ResetTimingType_Never,
		Unlocked:            true,
		ColorCode:           showcaseColorCode,
		BackgroundColorCode: showcaseBackgroundColorCode,
	}
}

func showcaseFrameCost(frameID string) (int32, bool) {
	switch frameID {
	case "frame-002":
		return 100, true
	case "frame-003":
		return 150, true
	case "frame-004":
		return 200, true
	default:
		return 0, false
	}
}
