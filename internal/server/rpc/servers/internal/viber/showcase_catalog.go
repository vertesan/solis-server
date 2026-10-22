package viber

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

const (
	ShowcaseGoodsShopID   = "shop_top_shop-showcase-goods-catalog"
	ShowcaseBadgeShopID   = "shop_top_shop-showcase-buttonbadge"
	ShowcaseFrameShopID   = "shop_top_shop-showcase-frame"
	ShowcaseMusicShopID   = "shop_top_shop-showcase-music"
	ShowcaseMedalItemID   = "item-showcase_medal"
	showcaseGoodsItemCost = int32(100)
)

const (
	showcaseColorCode           = "71BDFD"
	showcaseBackgroundColorCode = "3388D1"
	ShowcaseItemIDPrefix        = "shop_showcase_catalog_"
	ShowcaseMusicItemIDPrefix   = ShowcaseMusicShopID + "_"
	showcaseFrameItemIDPrefix   = "item-showcase_"
)

type ShowcaseCommerceCatalog struct {
	ShopEntries []*smodel.ShopCatalogEntry
	ShopsByID   map[string]*smodel.ShopCatalogEntry
	ItemsByID   map[string]showcaseCommerceItem
}

type showcaseCommerceItem struct {
	Shop *smodel.ShopCatalogEntry
	Item *papi.ShopItem
}

func BuildShowcaseCommerceCatalog(ctx context.Context, suid string) (*ShowcaseCommerceCatalog, error) {
	return buildShowcaseCommerceCatalog(ctx, suid, false)
}

// FindAvailableShowcaseCommerceShops uses the same item eligibility rules as
// Shop.ListItem, but only materializes the first item in each shop. Home needs
// its entrances, not hundreds of merchandise descriptions or their notices.
func FindAvailableShowcaseCommerceShops(ctx context.Context, suid string) ([]*smodel.ShopCatalogEntry, error) {
	catalog, err := buildShowcaseCommerceCatalog(ctx, suid, true)
	if err != nil {
		return nil, err
	}
	return catalog.Shops(), nil
}

func buildShowcaseCommerceCatalog(ctx context.Context, suid string, firstOnly bool) (*ShowcaseCommerceCatalog, error) {
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

	itemCapacity := len(toys) + len(frames) + len(showcaseMusics)
	if firstOnly {
		itemCapacity = 4
	}
	catalog := &ShowcaseCommerceCatalog{
		ShopsByID: make(map[string]*smodel.ShopCatalogEntry, 4),
		ItemsByID: make(map[string]showcaseCommerceItem, itemCapacity),
	}
	catalog.addShop(buildShowcaseGoodsShop(toys, ownedToyIDs, firstOnly))
	catalog.addShop(buildShowcaseBadgeShop(toys, firstOnly))
	catalog.addShop(buildShowcaseFrameShop(frames, ownedItemIDs, firstOnly))
	musicShop, err := buildShowcaseMusicShop(showcaseMusics, musicNames, ownedMusicIDs, firstOnly)
	if err != nil {
		return nil, err
	}
	catalog.addShop(musicShop)
	if firstOnly {
		return catalog, nil
	}
	notice, err := loadShowcaseShopNotice(ctx)
	if err != nil {
		return nil, err
	}
	for _, shop := range catalog.ShopEntries {
		// Official Showcase.Top supplies a notice even for permanent shops.
		// Keep it present before the client opens the shop's item screen.
		shop.Info.NoticeInfo = proto.Clone(notice).(*papi.NoticeInfo)
	}
	return catalog, nil
}

func loadShowcaseShopNotice(ctx context.Context) (*papi.NoticeInfo, error) {
	catalog, err := opsolis.FindNoticeCatalog(ctx)
	if errors.Is(err, mongo.ErrNoDocuments) || (err == nil && catalog.ShowcaseShopNotice == nil) {
		// Clearing notices must not leave a nil field that freezes the client.
		return NewDummyNoticeInfo(), nil
	}
	if err != nil {
		return nil, err
	}
	return catalog.ShowcaseShopNotice, nil
}

func IsShowcaseCommerceShopID(shopID string) bool {
	switch shopID {
	case ShowcaseGoodsShopID, ShowcaseBadgeShopID, ShowcaseFrameShopID, ShowcaseMusicShopID:
		return true
	default:
		return false
	}
}

func (c *ShowcaseCommerceCatalog) Shops() []*smodel.ShopCatalogEntry {
	if c == nil {
		return nil
	}
	return append([]*smodel.ShopCatalogEntry(nil), c.ShopEntries...)
}

func (c *ShowcaseCommerceCatalog) addShop(shop *smodel.ShopCatalogEntry) {
	if shop == nil || shop.Info == nil || len(shop.Items) == 0 {
		return
	}
	c.ShopEntries = append(c.ShopEntries, shop)
	c.ShopsByID[shop.ShopId] = shop
	for _, item := range shop.Items {
		if item != nil {
			c.ItemsByID[item.GetId()] = showcaseCommerceItem{Shop: shop, Item: item}
		}
	}
}

func buildShowcaseGoodsShop(toys []*pmaster.ShowcaseToy, ownedToyIDs map[string]struct{}, firstOnly bool) *smodel.ShopCatalogEntry {
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
			ShowcaseItemIDPrefix+toy.GetId(),
			toy.GetName(),
			toy.GetDescription(),
			"rd_auto;showcase_toy;"+toy.GetId()+";1",
			showcaseGoodsItemCost,
			toy.GetOrder(),
		))
		if firstOnly {
			break
		}
	}
	return newShowcaseShopCatalogEntry(
		ShowcaseGoodsShopID, "グッズカタログ", penum.ShopType_ShopType_DefaultShowcaseToyShop,
		// Preserve the literal "null" asset sentinel from official traffic.
		101, "null", items,
	)
}

func buildShowcaseBadgeShop(toys []*pmaster.ShowcaseToy, firstOnly bool) *smodel.ShopCatalogEntry {
	// DefaultShowcaseToyShop is the catalog, not a regular shop-list entry.
	// Keep the captured button-badge entrance available even when every unique
	// frame and BGM is owned. Badges can be acquired repeatedly, so this shop
	// never becomes empty merely because the user has been promoted to god.
	items := make([]*papi.ShopItem, 0)
	for _, toy := range toys {
		if toy.GetId() == "" || toy.GetCategoryId() != "toy-category-buttonbadge" {
			continue
		}
		items = append(items, newShowcaseShopItem(
			ShowcaseItemIDPrefix+"buttonbadge_"+toy.GetId(),
			toy.GetName(),
			toy.GetDescription(),
			"rd_auto;showcase_toy;"+toy.GetId()+";1",
			showcaseGoodsItemCost,
			toy.GetOrder(),
		))
		if firstOnly {
			break
		}
	}
	return newShowcaseShopCatalogEntry(
		ShowcaseBadgeShopID, "缶バッジショップ", penum.ShopType_ShopType_OnlyShowcaseShop,
		102, "showcase-buttonbadge", items,
	)
}

func buildShowcaseFrameShop(frames []*pmaster.ShowcaseFrame, ownedItemIDs map[string]struct{}, firstOnly bool) *smodel.ShopCatalogEntry {
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
			ShowcaseItemIDPrefix+"toy-goods-"+frame.GetId(),
			"ケース："+frame.GetName(),
			"ケース："+frame.GetDescription(),
			"rd_auto;item;"+itemID+";1",
			cost,
			10_000+frame.GetOrder(),
		)
		shopItem.ThumbnailAssetId = "img_shop_item_thumb_toy-goods-" + frame.GetId()
		items = append(items, shopItem)
		if firstOnly {
			break
		}
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
	firstOnly bool,
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
		// Retain catalog validation even when only the entrance is requested.
		if firstOnly && len(items) > 0 {
			continue
		}
		items = append(items, newShowcaseShopItem(
			ShowcaseMusicItemIDPrefix+musicID,
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
