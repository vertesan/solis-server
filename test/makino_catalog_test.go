package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/catalog"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Each catalog change is observed through the already-running server. Raw BSON
// snapshots restore the complete global documents, including IDs and nulls.
func TestMakinoCatalogScenario(t *testing.T) {
	backupMakinoCatalogCollections(t, sname.NoticeCatalog, sname.ShopCatalog, sname.ExchangeCatalog)
	setupTestUser(t)
	ctx := context.Background()
	apply := func(kind, mode string, documents ...[]byte) {
		t.Helper()
		if err := catalog.Apply(ctx, kind, mode, documents); err != nil {
			t.Fatalf("%s %s: %v", kind, mode, err)
		}
	}
	export := func(kind string) []byte {
		t.Helper()
		data, err := catalog.Export(ctx, kind)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	reject := func(kind, mode string, documents ...[]byte) {
		t.Helper()
		before := export(kind)
		if err := catalog.Apply(ctx, kind, mode, documents); err == nil {
			t.Fatalf("invalid %s %s import was accepted", kind, mode)
		}
		if after := export(kind); !bytes.Equal(before, after) {
			t.Fatalf("rejected %s import modified catalog", kind)
		}
	}

	apply("notice", "overwrite", []byte(`notices:
  - id: makino-test-notice
    title: First notice
    linkType: LinkType_WebView
    linkDetail: https://example.invalid/notice
    startTime: "9007199254740993"
showcaseShopNotice:
  id: makino-test-showcase-notice
  title: Goods notice
  linkType: LinkType_WebView
`))
	noticeClient := papi.NewNoticeClient(testConfig.GrpcConn)
	notices := profileCall(t, noticeClient.List, &papi.Empty{})
	if len(notices.GetNotices()) != 1 || notices.Notices[0].GetTitle() != "First notice" {
		t.Fatalf("notice change is not live: %v", notices)
	}
	if got := profileCall(t, noticeClient.Get, &papi.NoticeGetRequest{NoticeId: "makino-test-notice"}); got.GetNotice().GetTitle() != "First notice" {
		t.Fatalf("Notice.Get ignored the current catalog: %v", got)
	}
	showcaseClient := papi.NewShowcaseClient(testConfig.GrpcConn)
	showcase := profileCall(t, showcaseClient.Top, &papi.Empty{})
	if len(showcase.GetShops()) == 0 {
		t.Fatal("no showcase shops to check managed notice")
	}
	for _, shop := range showcase.GetShops() {
		if shop.GetNoticeInfo().GetTitle() != "Goods notice" {
			t.Fatalf("showcase notice did not update live: %v", shop)
		}
	}
	// Updating one category preserves the independent showcase notice and allows
	// an ID to move between notification tabs without duplication.
	apply("notice", "add", []byte("malfunctionNotices:\n  - id: makino-test-notice\n    title: Updated notice\n"), []byte("id: makino-test-second\ntitle: Second notice\n"))
	notices = profileCall(t, noticeClient.List, &papi.Empty{})
	if len(notices.GetNotices()) != 1 || len(notices.GetMalfunctionNotices()) != 1 || notices.MalfunctionNotices[0].GetTitle() != "Updated notice" {
		t.Fatalf("notice upsert duplicated/lost tabs: %v", notices)
	}
	storedNotice, err := opsolis.FindNoticeCatalog(ctx)
	if err != nil || storedNotice.ShowcaseShopNotice.GetId() != "makino-test-showcase-notice" {
		t.Fatalf("notice add lost showcase notice: %v", err)
	}
	// Round trips preserve large integer timestamps without float64 rounding.
	apply("notice", "add", []byte("id: makino-test-large-time\nstartTime: 9007199254740993\n"))
	apply("notice", "overwrite", export("notice"))
	storedNotice, err = opsolis.FindNoticeCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if storedNotice.Notices[len(storedNotice.Notices)-1].GetStartTime() != 9007199254740993 {
		t.Fatal("notice YAML round trip lost integer precision")
	}
	reject("notice", "overwrite", []byte("id: would-replace-catalog\n"), []byte("id: bad\ntitel: misspelled\n"))
	reject("notice", "add", []byte("id: duplicate\nid: duplicate\n"))
	reject("notice", "add", []byte("id: duplicate\n---\nid: duplicate\n"))
	reject("notice", "overwrite", []byte("notices: []\n"))
	reject("notice", "clear", []byte("id: unintended\n"))
	apply("notice", "clear")
	notices = profileCall(t, noticeClient.List, &papi.Empty{})
	if len(notices.GetNotices())+len(notices.GetMalfunctionNotices())+len(notices.GetPrNotices()) != 0 {
		t.Fatal("cleared notices were repopulated")
	}
	storedNotice, err = opsolis.FindNoticeCatalog(ctx)
	if err != nil || storedNotice.ShowcaseShopNotice != nil {
		t.Fatalf("notice clear did not explicitly clear showcase notice: %v", err)
	}
	assertDummyNoticeInfo(t, "cleared Notice.Get", profileCall(t, noticeClient.Get, &papi.NoticeGetRequest{NoticeId: "makino-test-notice"}).GetNotice())
	for _, shop := range profileCall(t, showcaseClient.Top, &papi.Empty{}).GetShops() {
		assertDummyNoticeInfo(t, "cleared showcase notice", shop.GetNoticeInfo())
	}

	// The unedited shipped catalogs must be valid editing starting points too.
	apply("shop", "overwrite", export("shop"))
	apply("exchange", "overwrite", export("exchange"))
	originalShops, err := opsolis.FindAllShopCatalogEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rewardID := ""
	productID := ""
	for _, shop := range originalShops {
		for _, item := range shop.Items {
			if item.GetRewardId() != "" {
				if _, err := opmaster.FindReward(ctx, item.GetRewardId()); err == nil {
					rewardID = item.GetRewardId()
				}
			}
		}
		for _, id := range shop.OriginalProductIds {
			if product, err := opmaster.FindStoreProduct(ctx, id); err == nil && product.GetStoneAmount() > 0 {
				productID = id
			}
		}
	}
	if rewardID == "" {
		t.Fatal("shop fixture needs a current master reward")
	}
	// Replacing a complete shop entry must also remove optional package state.
	packItem := &papi.ShopItem{Id: "item-makino-pack", Name: "Temporary package"}
	packShop := &smodel.ShopCatalogEntry{
		ShopId: "shop-makino-pack", Info: &papi.ShopInfo{Id: "shop-makino-pack", Name: "Package", Type: penum.ShopType_ShopType_LoginBonusShop},
		Items: []*papi.ShopItem{packItem}, LoginBonusPackageItem: &papi.LoginBonusPackageItem{ShopItem: packItem},
	}
	apply("shop", "overwrite", makinoCatalogJSON(t, packShop))
	packShop.LoginBonusPackageItem = nil
	packShop.Info.Type = penum.ShopType_ShopType_NormalShop
	apply("shop", "add", makinoCatalogJSON(t, packShop))
	clearedPack, err := opsolis.FindShopCatalogEntry(ctx, packShop.ShopId)
	if err != nil || clearedPack.LoginBonusPackageItem != nil {
		t.Fatalf("replacement retained removed login package: %v", err)
	}
	shop := &smodel.ShopCatalogEntry{
		ShopId: "shop-makino-test", Info: &papi.ShopInfo{Id: "shop-makino-test", Name: "Managed shop", Type: penum.ShopType_ShopType_NormalShop},
		Items: []*papi.ShopItem{{Id: "item-makino-test", Name: "Managed item", RewardId: rewardID, ProductId: productID, PurchaseLimit: 3, LeftCount: 2}},
	}
	apply("shop", "overwrite", makinoCatalogJSON(t, shop))
	shopClient := papi.NewShopClient(testConfig.GrpcConn)
	shops := profileCall(t, shopClient.List, &papi.Empty{})
	if len(shops.GetShops()) != 1 || shops.Shops[0].GetName() != "Managed shop" {
		t.Fatalf("new shop not available without restart: %v", shops)
	}
	items := profileCall(t, shopClient.ListItem, &papi.ShopListItemRequest{ShopId: shop.ShopId})
	if len(items.GetShopItems()) != 1 || items.ShopItems[0].GetName() != "Managed item" || items.ShopItems[0].GetProductId() != "" || items.ShopItems[0].GetPurchaseLimit() != 0 {
		t.Fatalf("shop normalization not applied: %v", items)
	}
	storedShop, err := opsolis.FindShopCatalogEntry(ctx, shop.ShopId)
	if err != nil {
		t.Fatal(err)
	}
	if productID != "" && storedShop.OriginalProductIds["item-makino-test"] != productID {
		t.Fatal("converted bundle lost original diamond mapping")
	}
	storedShop.Items[0].Name = "Edited without restart"
	apply("shop", "add", makinoCatalogJSON(t, storedShop))
	items = profileCall(t, shopClient.ListItem, &papi.ShopListItemRequest{ShopId: shop.ShopId})
	if items.ShopItems[0].GetName() != "Edited without restart" {
		t.Fatal("shop update was cached")
	}
	// Inline rewards are not master Reward IDs; imports and purchases must
	// interpret the same format, including converted paid-diamond bundles.
	storedShop.Items[0].RewardId = "rd_auto;paid_stone;;160"
	storedShop.OriginalProductIds = nil
	apply("shop", "add", makinoCatalogJSON(t, storedShop))
	clearedMapping, err := opsolis.FindShopCatalogEntry(ctx, storedShop.ShopId)
	if err != nil || len(clearedMapping.OriginalProductIds) != 0 {
		t.Fatalf("replacement retained removed product mapping: %v", err)
	}
	balanceBefore, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	profileCall(t, shopClient.Purchase, &papi.ShopPurchaseRequest{ShopItemId: storedShop.Items[0].GetId()})
	balanceAfter, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	if err != nil || balanceAfter.GetPaidBalance() != balanceBefore.GetPaidBalance()+160 || balanceAfter.GetFreeBalance() != balanceBefore.GetFreeBalance() {
		t.Fatalf("imported inline reward did not grant exactly 160 paid diamonds: before=%v after=%v err=%v", balanceBefore, balanceAfter, err)
	}
	apply("shop", "overwrite", export("shop"))
	reject("shop", "overwrite", []byte("shopId: empty\ninfo: {id: empty}\nitems: []\n"))
	reject("shop", "overwrite", []byte("shopId: typo\ninfo: {id: typo, titel: wrong}\nitems: []\n"))
	reject("shop", "add", []byte("shopId: collision\ninfo: {id: collision}\nitems: [{id: item-makino-test}]\n"))
	reject("shop", "overwrite", []byte("shopId: invalid\ninfo: {id: invalid}\nitems: [{id: invalid-item, rewardId: no-such-makino-reward}]\n"))
	for _, rewardID := range []string{"rd_auto;paid_stone;;-1", "rd_auto;paid_stone;;not-a-number", "rd_auto;unsupported;;1", "rd_auto;paid_stone;"} {
		storedShop.Items[0].RewardId = rewardID
		reject("shop", "overwrite", makinoCatalogJSON(t, storedShop))
	}
	apply("shop", "clear")
	if shops = profileCall(t, shopClient.List, &papi.Empty{}); len(shops.GetShops()) != 0 {
		t.Fatal("cleared shops were repopulated")
	}

	booth := &papi.ExchangeBooth{
		Id: "exchange-makino-test", Name: "Managed exchange", RequiredResourceType: penum.ResourceType_ResourceType_Stone,
		Exchanges: []*papi.ExchangeItem{{Id: "exchange-item-makino-test", Name: "Managed reward", RewardId: rewardID, RequiredResourceAmount: 5}},
	}
	apply("exchange", "overwrite", makinoCatalogJSON(t, booth))
	exchangeClient := papi.NewExchangeClient(testConfig.GrpcConn)
	booths := profileCall(t, exchangeClient.List, &papi.Empty{})
	if len(booths.GetBooths()) != 1 || booths.Booths[0].GetName() != "Managed exchange" {
		t.Fatalf("exchange change not live: %v", booths)
	}
	booth.Exchanges[0].RequiredResourceAmount = 7
	apply("exchange", "add", makinoCatalogJSON(t, booth))
	booths = profileCall(t, exchangeClient.List, &papi.Empty{})
	if booths.Booths[0].Exchanges[0].GetRequiredResourceAmount() != 7 {
		t.Fatal("exchange cost change was cached")
	}
	apply("exchange", "overwrite", export("exchange"))
	booth.Exchanges[0].RequiredResourceAmount = -1
	reject("exchange", "overwrite", makinoCatalogJSON(t, booth))
	booth.Exchanges[0].RequiredResourceAmount = 1
	booth.Id = "exchange-collision"
	reject("exchange", "add", makinoCatalogJSON(t, booth))
	apply("exchange", "clear")
	if booths = profileCall(t, exchangeClient.List, &papi.Empty{}); len(booths.GetBooths()) != 0 {
		t.Fatal("cleared exchanges were repopulated")
	}
}

func makinoCatalogJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data // JSON is also valid YAML and exercises the same strict decoder.
}

func backupMakinoCatalogCollections(t *testing.T, names ...string) {
	t.Helper()
	snapshots := make(map[string][]bson.Raw, len(names))
	err := transaction.WithTransaction(context.Background(), func(ctx context.Context) (any, error) {
		database := mongo.SessionFromContext(ctx).Client().Database("solis")
		for _, name := range names {
			cursor, err := database.Collection(name).Find(ctx, bson.M{})
			if err != nil {
				return nil, err
			}
			var documents []bson.Raw
			if err := cursor.All(ctx, &documents); err != nil {
				return nil, err
			}
			snapshots[name] = documents
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("snapshot catalogs: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
			database := mongo.SessionFromContext(ctx).Client().Database("solis")
			for _, name := range names {
				collection := database.Collection(name)
				if _, err := collection.DeleteMany(ctx, bson.M{}); err != nil {
					return nil, err
				}
				if documents := snapshots[name]; len(documents) != 0 {
					if _, err := collection.InsertMany(ctx, documents); err != nil {
						return nil, err
					}
				}
			}
			return nil, nil
		})
		if err != nil {
			t.Error(fmt.Errorf("restore catalogs: %w", err))
		}
	})
}
