package test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

func TestShowcaseScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	client := papi.NewShowcaseClient(testConfig.GrpcConn)
	shopClient := papi.NewShopClient(testConfig.GrpcConn)
	// Check the frame/music entrances before god promotion owns every unique
	// item and removes those shops. The client consumes this metadata before
	// sending Shop.ListItem, so successful purchases alone cannot catch it.
	freshTop := showcaseTop(t, client)
	assertShowcaseShopPresentation(t, freshTop.GetShops())
	assertShowcaseRepeatedTop(t, freshTop, showcaseTop(t, client))
	assertHomeShowcaseShopRoster(t, freshTop.GetShops(), false)
	promoteTestUser(t)

	top := showcaseTop(t, client)
	assertShowcaseShopPresentation(t, top.GetShops())
	assertHomeShowcaseShopRoster(t, top.GetShops(), true)
	// Opening the shop list invokes Top a second time. Owning all unique
	// frames/BGM must not leave only the special catalog type (1003).
	assertShowcaseRepeatedTop(t, top, showcaseTop(t, client))
	for _, shop := range top.GetShops() {
		if shop.GetId() == "shop_top_shop-showcase-frame" || shop.GetId() == "shop_top_shop-showcase-music" {
			t.Fatalf("god user's fully owned unique shop should be hidden: %v", shop)
		}
	}
	if len(top.GetShowcaseInfos()) == 0 || len(top.GetMysetInfos()) == 0 ||
		len(top.GetFrameInfos()) == 0 || len(top.GetShowcaseMusicIds()) == 0 {
		t.Fatalf("Showcase.Top returned incomplete master-driven data: %v", top)
	}
	assertShowcaseMusicCatalog(t, dbCtx, top.GetShowcaseMusicIds())

	var selectedCase *papi.ShowcaseSelfInfo
	for _, info := range top.GetShowcaseInfos() {
		if info.GetNumber() <= 0 || info.GetDefaultName() == "" || info.GetLayoutInfo() == nil {
			t.Fatalf("Showcase.Top returned an invalid case slot: %v", info)
		}
		if selectedCase == nil && info.GetIsUnlocked() {
			selectedCase = info
		}
	}
	if selectedCase == nil {
		t.Fatal("fresh test user has no unlocked showcase slot")
	}
	var selectedFrameID string
	for _, frame := range top.GetFrameInfos() {
		if frame.GetId() == "" {
			t.Fatalf("Showcase.Top returned a frame without an ID: %v", frame)
		}
		if selectedFrameID == "" && frame.GetIsUnlocked() {
			selectedFrameID = frame.GetId()
		}
	}
	if selectedFrameID == "" {
		t.Fatal("fresh test user has no unlocked showcase frame")
	}

	masterToys, err := opmaster.FindAllShowcaseToys(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	masterToyByID := make(map[string]*pmaster.ShowcaseToy, len(masterToys))
	for _, toy := range masterToys {
		masterToyByID[toy.GetId()] = toy
	}
	characters, err := opuser.FindAllUserCharacters(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(characters) == 0 {
		t.Fatal("fresh test user has no character for showcase shooting")
	}
	ownedCharacterIDs := make(map[string]struct{}, len(characters))
	for _, character := range characters {
		ownedCharacterIDs[character.GetCharacterId()] = struct{}{}
	}

	var goodsItems, badgeItems []*papi.ShopItem
	for _, shop := range top.GetShops() {
		items := listShopItems(t, shopClient, shop.GetId()).GetShopItems()
		if len(items) == 0 {
			t.Fatalf("Showcase.Top advertised shop %q with no items", shop.GetId())
		}
		if shop.GetId() == "shop_top_shop-showcase-goods-catalog" {
			goodsItems = items
		}
		if shop.GetId() == "shop_top_shop-showcase-buttonbadge" {
			badgeItems = items
		}
	}
	if len(goodsItems) == 0 {
		t.Fatal("Showcase.Top did not advertise a nonempty goods catalog")
	}
	assertShowcaseBadgeCatalog(t, badgeItems, masterToyByID)

	shopItem, selectedToy, focusCharacterID := selectShowcaseToyPurchase(
		t, badgeItems, masterToyByID, ownedCharacterIDs,
	)
	cost, ok := showcaseMedalConsumption(shopItem.GetConsumptionId())
	if !ok || cost <= 0 {
		t.Fatalf("selected ShowcaseToy item has an unsupported consumption: %v", shopItem)
	}
	const showcaseMedalID = "item-showcase_medal"
	seedAmount := 2*cost + 1000
	if err := opuser.UpsertUserItem(dbCtx, testConfig.TestUserSuid, &ptransaction.UserItem{
		ItemId: showcaseMedalID,
		Amount: seedAmount,
	}); err != nil {
		t.Fatal(err)
	}
	toysBefore, err := opuser.FindAllUserShowcaseToys(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	toyAmountBefore := showcaseToyAmount(toysBefore, selectedToy.GetId())

	var purchase *papi.ShopPurchaseResponse
	for range 2 {
		purchaseCtx, purchaseCancel := showcaseGRPCCallContext(t)
		purchase, err = shopClient.Purchase(purchaseCtx, &papi.ShopPurchaseRequest{ShopItemId: shopItem.GetId()})
		purchaseCancel()
		if err != nil {
			t.Fatalf("Shop.Purchase(%q) failed: %v", shopItem.GetId(), err)
		}
		if purchase.GetCommonResponse().GetUpdatedData() == nil ||
			purchase.GetCommonResponse().GetDeletedData() == nil {
			t.Fatal("ShowcaseToy purchase returned an incomplete common response")
		}
	}
	storedToy, err := opuser.FindUserShowcaseToy(dbCtx, testConfig.TestUserSuid, selectedToy.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if storedToy.GetAmount() != toyAmountBefore+2 {
		t.Fatalf("ShowcaseToy amount = %d, want %d", storedToy.GetAmount(), toyAmountBefore+2)
	}
	if got := photoPermanentItemAmount(t, dbCtx, showcaseMedalID); got != seedAmount-2*cost {
		t.Fatalf("showcase medal amount = %d, want %d", got, seedAmount-2*cost)
	}
	if showcaseToyAmount(purchase.GetCommonResponse().GetUpdatedData().GetShowcaseToys(), selectedToy.GetId()) != storedToy.GetAmount() {
		t.Fatalf("Shop.Purchase omitted ShowcaseToy post-image: %v", purchase)
	}

	showcaseFaceNumber := int32(0)
	if !slices.Contains(selectedToy.GetPutTypes(), penum.ShowcaseToyPutType_ShowcaseToyPutType_Floor) {
		showcaseFaceNumber = 1
	}
	layout := &papi.ShowcaseLayoutInfo{
		ShowcaseToyAssetInfos: []*papi.ShowcaseToyAssetInfo{{
			UniqueNumber:  1,
			ShowcaseToyId: selectedToy.GetId(),
			Count:         1,
		}},
		ShowcaseToyArrangementInfos: []*papi.ShowcaseToyArrangementInfo{{
			UniqueNumber:             1,
			ToyAssetInfoUniqueNumber: 1,
			ShowcaseFaceNumber:       showcaseFaceNumber,
		}},
	}

	thumbnailCheckCtx, thumbnailCheckCancel := showcaseGRPCCallContext(t)
	thumbnailCheck, err := client.CheckThumbnailShooting(thumbnailCheckCtx, &papi.ShowcaseCheckThumbnailShootingRequest{
		Number: selectedCase.GetNumber(),
	})
	thumbnailCheckCancel()
	if err != nil {
		t.Fatalf("Showcase.CheckThumbnailShooting failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.CheckThumbnailShooting", thumbnailCheck.GetCommonResponse())

	userBeforeSave, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(userBeforeSave.GetNextPhotoImageIds()) == 0 {
		t.Fatal("fresh test user has no reserved thumbnail asset ID")
	}
	reservedBeforeSave := slices.Clone(userBeforeSave.GetNextPhotoImageIds())
	thumbnailAssetID := reservedBeforeSave[0]

	saveCtx, saveCancel := showcaseGRPCCallContext(t)
	save, err := client.Save(saveCtx, &papi.ShowcaseSaveRequest{
		Number:     selectedCase.GetNumber(),
		FrameId:    selectedFrameID,
		AssetId:    thumbnailAssetID,
		LayoutInfo: proto.Clone(layout).(*papi.ShowcaseLayoutInfo),
	})
	saveCancel()
	if err != nil {
		t.Fatalf("Showcase.Save failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.Save", save.GetCommonResponse())
	storedLayout, err := opuser.FindUserShowcaseLayout(dbCtx, testConfig.TestUserSuid, selectedCase.GetNumber())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(storedLayout, layout) {
		t.Fatalf("stored showcase layout differs from request: stored=%v request=%v", storedLayout, layout)
	}
	storedShowcase, err := opuser.FindUserShowcase(dbCtx, testConfig.TestUserSuid, selectedCase.GetNumber())
	if err != nil {
		t.Fatal(err)
	}
	if storedShowcase.GetThumbnailAssetId() != thumbnailAssetID ||
		storedShowcase.GetShowcaseFrameId() != selectedFrameID {
		t.Fatalf("Showcase.Save persisted wrong detail: %v", storedShowcase)
	}
	userAfterSave, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(userAfterSave.GetNextPhotoImageIds()) != len(reservedBeforeSave) ||
		slices.Contains(userAfterSave.GetNextPhotoImageIds(), thumbnailAssetID) ||
		!allPhotoImageIDsUnique(userAfterSave.GetNextPhotoImageIds()) {
		t.Fatalf("Showcase.Save did not rotate the thumbnail asset ID: before=%v after=%v", reservedBeforeSave, userAfterSave.GetNextPhotoImageIds())
	}
	topAfterSave := showcaseTop(t, client)
	persistedSlot := showcaseSelfInfo(topAfterSave.GetShowcaseInfos(), selectedCase.GetNumber())
	if persistedSlot == nil || !proto.Equal(persistedSlot.GetLayoutInfo(), layout) {
		t.Fatalf("Showcase.Top omitted saved layout: %v", persistedSlot)
	}

	const showcaseName = "Test Case"
	const showcaseComment = "integration"
	detailCtx, detailCancel := showcaseGRPCCallContext(t)
	detail, err := client.SaveDetail(detailCtx, &papi.ShowcaseSaveDetailRequest{
		Number:  selectedCase.GetNumber(),
		Name:    showcaseName,
		Comment: showcaseComment,
		MusicId: "",
		IsOpen:  true,
	})
	detailCancel()
	if err != nil {
		t.Fatalf("Showcase.SaveDetail failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.SaveDetail", detail.GetCommonResponse())
	storedShowcase, err = opuser.FindUserShowcase(dbCtx, testConfig.TestUserSuid, selectedCase.GetNumber())
	if err != nil {
		t.Fatal(err)
	}
	if storedShowcase.GetName() != showcaseName || storedShowcase.GetComment() != showcaseComment ||
		!storedShowcase.GetIsOpen() || storedShowcase.GetThumbnailAssetId() != thumbnailAssetID ||
		storedShowcase.GetShowcaseFrameId() != selectedFrameID {
		t.Fatalf("Showcase.SaveDetail persisted an invalid post-image: %v", storedShowcase)
	}

	mysetNumber := top.GetMysetInfos()[0].GetNumber()
	const mysetName = "Test Myset"
	renameCtx, renameCancel := showcaseGRPCCallContext(t)
	rename, err := client.RenameMyset(renameCtx, &papi.ShowcaseRenameMysetRequest{
		Number: mysetNumber,
		Name:   mysetName,
	})
	renameCancel()
	if err != nil {
		t.Fatalf("Showcase.RenameMyset failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.RenameMyset", rename.GetCommonResponse())

	saveMysetCtx, saveMysetCancel := showcaseGRPCCallContext(t)
	saveMyset, err := client.SaveMyset(saveMysetCtx, &papi.ShowcaseSaveMysetRequest{
		Number:     mysetNumber,
		FrameId:    selectedFrameID,
		LayoutInfo: proto.Clone(layout).(*papi.ShowcaseLayoutInfo),
	})
	saveMysetCancel()
	if err != nil {
		t.Fatalf("Showcase.SaveMyset failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.SaveMyset", saveMyset.GetCommonResponse())
	storedMyset, err := opuser.FindUserShowcaseMyset(dbCtx, testConfig.TestUserSuid, mysetNumber)
	if err != nil {
		t.Fatal(err)
	}
	if storedMyset.GetName() != mysetName || storedMyset.GetShowcaseFrameId() != selectedFrameID || !storedMyset.GetIsSave() {
		t.Fatalf("Showcase.SaveMyset persisted invalid metadata: %v", storedMyset)
	}

	applyCtx, applyCancel := showcaseGRPCCallContext(t)
	apply, err := client.ApplyMyset(applyCtx, &papi.ShowcaseApplyMysetRequest{Number: mysetNumber})
	applyCancel()
	if err != nil {
		t.Fatalf("Showcase.ApplyMyset failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.ApplyMyset", apply.GetCommonResponse())
	if apply.GetFrameId() != selectedFrameID || !proto.Equal(apply.GetLayoutInfo(), layout) {
		t.Fatalf("Showcase.ApplyMyset returned the wrong saved state: %v", apply)
	}

	deleteCtx, deleteCancel := showcaseGRPCCallContext(t)
	deleted, err := client.DeleteMyset(deleteCtx, &papi.ShowcaseDeleteMysetRequest{Number: mysetNumber})
	deleteCancel()
	if err != nil {
		t.Fatalf("Showcase.DeleteMyset failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.DeleteMyset", deleted.GetCommonResponse())
	if _, err := opuser.FindUserShowcaseMyset(dbCtx, testConfig.TestUserSuid, mysetNumber); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("Showcase.DeleteMyset left metadata behind: %v", err)
	}
	if _, err := opuser.FindUserShowcaseMysetLayout(dbCtx, testConfig.TestUserSuid, mysetNumber); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("Showcase.DeleteMyset left layout behind: %v", err)
	}

	assertEmptyShowcaseGallery(t, client)
	createShowcasePhoto(
		t, dbCtx, client, selectedCase.GetNumber(), showcaseName, focusCharacterID,
	)
}

func assertShowcaseMusicCatalog(t *testing.T, ctx context.Context, gotIDs []string) {
	t.Helper()
	masters, err := opmaster.FindAllShowcaseMusics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := make(map[string]struct{}, len(masters))
	for _, music := range masters {
		want[music.GetMusicId()] = struct{}{}
	}
	got := make(map[string]struct{}, len(gotIDs))
	for _, id := range gotIDs {
		if _, ok := want[id]; !ok {
			t.Fatalf("Showcase.Top returned music %q absent from ShowcaseMusic master", id)
		}
		got[id] = struct{}{}
	}
	if len(got) != len(want) {
		t.Fatalf("Showcase.Top returned %d unique musics, want %d from master", len(got), len(want))
	}
}

func selectShowcaseToyPurchase(
	t *testing.T,
	items []*papi.ShopItem,
	masters map[string]*pmaster.ShowcaseToy,
	ownedCharacterIDs map[string]struct{},
) (*papi.ShopItem, *pmaster.ShowcaseToy, string) {
	t.Helper()
	type candidate struct {
		item      *papi.ShopItem
		toy       *pmaster.ShowcaseToy
		character string
	}
	var fallback *candidate
	for _, item := range items {
		toyID, ok := showcaseToyRewardID(item.GetRewardId())
		if !ok {
			continue
		}
		toy := masters[toyID]
		if toy == nil {
			t.Fatalf("goods catalog item %q references missing ShowcaseToy %q", item.GetId(), toyID)
		}
		if _, ok := showcaseMedalConsumption(item.GetConsumptionId()); !ok {
			continue
		}
		current := &candidate{item: item, toy: toy}
		for _, characterID := range toy.GetPhotoShootingCharacterIds() {
			if _, owned := ownedCharacterIDs[characterID]; owned {
				current.character = characterID
				return current.item, current.toy, current.character
			}
		}
		if fallback == nil {
			fallback = current
		}
	}
	if fallback == nil {
		t.Fatal("goods catalog has no purchasable master-backed ShowcaseToy")
	}
	for characterID := range ownedCharacterIDs {
		fallback.character = characterID
		break
	}
	return fallback.item, fallback.toy, fallback.character
}

func showcaseToyRewardID(rewardID string) (string, bool) {
	parts := strings.Split(rewardID, ";")
	if len(parts) != 4 || parts[0] != "rd_auto" || parts[1] != "showcase_toy" || parts[2] == "" || parts[3] != "1" {
		return "", false
	}
	return parts[2], true
}

func showcaseMedalConsumption(consumptionID string) (int64, bool) {
	const prefix = "con-item_showcase-medal-"
	if !strings.HasPrefix(consumptionID, prefix) {
		return 0, false
	}
	amount, err := strconv.ParseInt(strings.TrimPrefix(consumptionID, prefix), 10, 64)
	return amount, err == nil && amount > 0
}

func showcaseToyAmount(toys []*ptransaction.UserShowcaseToy, toyID string) int64 {
	for _, toy := range toys {
		if toy.GetShowcaseToyId() == toyID {
			return toy.GetAmount()
		}
	}
	return 0
}

func showcaseTop(t *testing.T, client papi.ShowcaseClient) *papi.ShowcaseTopResponse {
	t.Helper()
	ctx, cancel := showcaseGRPCCallContext(t)
	response, err := client.Top(ctx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("Showcase.Top failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.Top", response.GetCommonResponse())
	return response
}

func assertShowcaseShopPresentation(t *testing.T, shops []*papi.ShopInfo) {
	t.Helper()
	if len(shops) == 0 {
		t.Fatal("Showcase.Top returned no goods catalog entrance")
	}
	hasRegularShop, hasBadgeShop := false, false
	for _, shop := range shops {
		notice := shop.GetNoticeInfo()
		if notice == nil || notice.GetId() == "" || notice.GetTitle() == "" ||
			notice.GetListTitle() == "" || notice.GetBannerAssetId() == "" ||
			notice.GetLinkType() != penum.LinkType_LinkType_WebView || notice.GetLinkDetail() == "" {
			t.Fatalf("Showcase.Top shop %q has incomplete pre-entry notice metadata: %v", shop.GetId(), notice)
		}
		if notice.GetDisplayNotification() {
			t.Fatalf("Showcase.Top shop %q enabled a permanent notice badge", shop.GetId())
		}
		if shop.GetId() == "shop_top_shop-showcase-goods-catalog" {
			if shop.GetType() != penum.ShopType_ShopType_DefaultShowcaseToyShop || shop.GetThumbnailAssetId() != "null" {
				t.Fatalf("Showcase.Top goods entrance differs from the captured client contract: %v", shop)
			}
		} else if shop.GetThumbnailAssetId() == "" {
			t.Fatalf("Showcase.Top shop %q has no thumbnail asset", shop.GetId())
		}
		if shop.GetType() == penum.ShopType_ShopType_OnlyShowcaseShop {
			hasRegularShop = true
		}
		if shop.GetId() == "shop_top_shop-showcase-buttonbadge" {
			hasBadgeShop = true
			if shop.GetType() != penum.ShopType_ShopType_OnlyShowcaseShop ||
				shop.GetOrder() != 102 || shop.GetThumbnailAssetId() != "showcase-buttonbadge" {
				t.Fatalf("Showcase.Top badge entrance differs from the official client contract: %v", shop)
			}
		}
	}
	if !hasRegularShop || !hasBadgeShop {
		t.Fatalf("Showcase.Top must retain a regular badge shop, not just the special goods catalog: %v", shops)
	}
}

func assertShowcaseRepeatedTop(t *testing.T, first, second *papi.ShowcaseTopResponse) {
	t.Helper()
	if len(first.GetShops()) != len(second.GetShops()) {
		t.Fatalf("opening the goods shop list changed the available shops: first=%v second=%v", first.GetShops(), second.GetShops())
	}
	for i, shop := range first.GetShops() {
		if !proto.Equal(shop, second.GetShops()[i]) {
			t.Fatalf("opening the goods shop list changed shop metadata: first=%v second=%v", shop, second.GetShops()[i])
		}
	}
}

// Home only builds an availability projection. It must match the full shop
// catalog both before and after ownership changes remove unique merchandise.
func assertHomeShowcaseShopRoster(t *testing.T, shops []*papi.ShopInfo, unlocked bool) {
	t.Helper()
	want := make([]string, 0, len(shops))
	for _, shop := range shops {
		want = append(want, shop.GetId())
	}
	for range 2 {
		ctx, cancel := showcaseGRPCCallContext(t)
		home, err := papi.NewHomeClient(testConfig.GrpcConn).Enter(ctx, &papi.HomeEnterRequest{})
		cancel()
		if err != nil {
			t.Fatalf("Home.Enter failed while checking Showcase entrances: %v", err)
		}
		stored, err := opuser.FindUserShowcaseToyShopIds(context.Background(), testConfig.TestUserSuid)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(stored.ShopIds, want) {
			t.Fatalf("Home and Showcase catalogs differ: home=%v showcase=%v", stored.ShopIds, want)
		}
		if unlocked && !slices.Equal(home.GetShowcaseToyShopIds(), want) {
			t.Fatalf("Home hid an available Showcase shop: home=%v showcase=%v", home.GetShowcaseToyShopIds(), want)
		}
	}
}

func assertShowcaseBadgeCatalog(t *testing.T, items []*papi.ShopItem, masters map[string]*pmaster.ShowcaseToy) {
	t.Helper()
	badges := make(map[string]struct{})
	for id, toy := range masters {
		if toy.GetCategoryId() == "toy-category-buttonbadge" {
			badges[id] = struct{}{}
		}
	}
	if len(items) == 0 || len(items) != len(badges) {
		t.Fatalf("badge shop has %d items, want %d from ShowcaseToy master", len(items), len(badges))
	}
	for _, item := range items {
		toyID, ok := showcaseToyRewardID(item.GetRewardId())
		if !ok {
			t.Fatalf("badge shop has an invalid toy reward: %v", item)
		}
		if _, exists := badges[toyID]; !exists {
			t.Fatalf("badge shop has a duplicate or non-badge reward: %v", item)
		}
		delete(badges, toyID)
		if !item.GetUnlocked() || item.GetPurchaseLimit() != 0 || item.GetProductId() != "" {
			t.Fatalf("badge shop should offer repeatable non-cash purchases: %v", item)
		}
	}
}

func showcaseSelfInfo(infos []*papi.ShowcaseSelfInfo, number int32) *papi.ShowcaseSelfInfo {
	for _, info := range infos {
		if info.GetNumber() == number {
			return info
		}
	}
	return nil
}

func assertEmptyShowcaseGallery(t *testing.T, client papi.ShowcaseClient) {
	t.Helper()
	galleryCtx, galleryCancel := showcaseGRPCCallContext(t)
	gallery, err := client.GalleryTop(galleryCtx, &papi.Empty{})
	galleryCancel()
	if err != nil {
		t.Fatalf("Showcase.GalleryTop failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.GalleryTop", gallery.GetCommonResponse())
	if len(gallery.GetShowcaseInfos()) != 0 {
		t.Fatalf("private Showcase.GalleryTop returned multiplayer entries: %v", gallery.GetShowcaseInfos())
	}

	friendsCtx, friendsCancel := showcaseGRPCCallContext(t)
	friends, err := client.SearchFriendRecommendList(friendsCtx, &papi.Empty{})
	friendsCancel()
	if err != nil {
		t.Fatalf("Showcase.SearchFriendRecommendList failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.SearchFriendRecommendList", friends.GetCommonResponse())
	if len(friends.GetShowcaseInfos()) != 0 {
		t.Fatalf("private Showcase.SearchFriendRecommendList returned multiplayer entries: %v", friends.GetShowcaseInfos())
	}

	hashtagCtx, hashtagCancel := showcaseGRPCCallContext(t)
	hashtag, err := client.SearchHashtagList(hashtagCtx, &papi.ShowcaseSearchHashtagListRequest{
		HashtagId: "hashtag-char-skr",
		IsHead:    true,
	})
	hashtagCancel()
	if err != nil {
		t.Fatalf("Showcase.SearchHashtagList failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.SearchHashtagList", hashtag.GetCommonResponse())
	if len(hashtag.GetShowcaseInfos()) != 0 || !hashtag.GetIsEndHashtagSearchList() || !hashtag.GetIsAlwaysEmpty() {
		t.Fatalf("private Showcase.SearchHashtagList returned a nonempty/nonterminal result: %v", hashtag)
	}

	getCtx, getCancel := showcaseGRPCCallContext(t)
	got, err := client.Get(getCtx, &papi.ShowcaseGetRequest{UserId: "ZZZZZZZZ"})
	getCancel()
	if err != nil {
		t.Fatalf("Showcase.Get dummy target failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.Get", got.GetCommonResponse())
	if len(got.GetShowcaseInfos()) != 0 {
		t.Fatalf("private Showcase.Get returned multiplayer entries: %v", got.GetShowcaseInfos())
	}

	displayCtx, displayCancel := showcaseGRPCCallContext(t)
	display, err := client.Display(displayCtx, &papi.ShowcaseDisplayRequest{UserId: "ZZZZZZZZ", Number: 1})
	displayCancel()
	if err != nil {
		t.Fatalf("Showcase.Display dummy target failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.Display", display.GetCommonResponse())
	if display.GetIsViewable() || display.GetShowcaseInfo() != nil || display.GetLayoutInfo() != nil {
		t.Fatalf("private Showcase.Display returned multiplayer state: %v", display)
	}
}

func createShowcasePhoto(
	t *testing.T,
	ctx context.Context,
	client papi.ShowcaseClient,
	showcaseNumber int32,
	showcaseName string,
	focusCharacterID string,
) {
	t.Helper()

	checkCtx, checkCancel := showcaseGRPCCallContext(t)
	check, err := client.CheckShooting(checkCtx, &papi.ShowcaseCheckShootingRequest{Number: showcaseNumber})
	checkCancel()
	if err != nil {
		t.Fatalf("Showcase.CheckShooting failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.CheckShooting", check.GetCommonResponse())

	userBefore, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(userBefore.GetNextPhotoImageIds()) == 0 {
		t.Fatal("test user has no reserved Showcase photo image ID")
	}
	reservedBefore := slices.Clone(userBefore.GetNextPhotoImageIds())
	usedAssetID := reservedBefore[0]
	const filmID = "item-photo_shooting-01"
	const filmAmount int64 = 2
	if err := opuser.UpsertUserItem(ctx, testConfig.TestUserSuid, &ptransaction.UserItem{
		ItemId: filmID,
		Amount: filmAmount,
	}); err != nil {
		t.Fatal(err)
	}
	characterBefore, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, focusCharacterID)
	if err != nil {
		t.Fatal(err)
	}
	totalBefore, err := opuser.FindUserTotalCount(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}

	createCtx, createCancel := showcaseGRPCCallContext(t)
	created, err := client.CreatePhoto(createCtx, &papi.ShowcaseCreatePhotoRequest{
		Number: showcaseNumber,
		CreatePhotoParams: []*papi.ShowcaseCreatePhotoParam{{
			AssetId:         usedAssetID,
			ItemId:          filmID,
			MainCharacterId: focusCharacterID,
		}},
		ManualCount: 1,
	})
	createCancel()
	if err != nil {
		t.Fatalf("Showcase.CreatePhoto failed: %v", err)
	}
	assertShowcaseCommonResponse(t, "Showcase.CreatePhoto", created.GetCommonResponse())
	if created.GetIsGift() || len(created.GetPhotos()) != 1 {
		t.Fatalf("Showcase.CreatePhoto returned an unexpected result: %v", created)
	}
	photo := created.GetPhotos()[0]
	if photo.GetPhotoId() == "" || photo.GetAssetId() != usedAssetID ||
		photo.GetImageType() != penum.PhotoImageType_PhotoImageType_User ||
		photo.GetFocusCharacterId() != focusCharacterID || photo.GetPlaceName() != showcaseName ||
		photo.GetEventName() != "グッズケース" {
		t.Fatalf("Showcase.CreatePhoto returned invalid photo identity/scene: %v", photo)
	}
	assertGeneratedPhotoIsMasterValid(t, ctx, photo)
	stored, err := opuser.FindUserPhoto(ctx, testConfig.TestUserSuid, photo.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(stored, photo) {
		t.Fatalf("stored Showcase photo differs from response: stored=%v response=%v", stored, photo)
	}
	if got := photoPermanentItemAmount(t, ctx, filmID); got != filmAmount-1 {
		t.Fatalf("Showcase photo film amount = %d, want %d", got, filmAmount-1)
	}
	characterAfter, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, focusCharacterID)
	if err != nil {
		t.Fatal(err)
	}
	if characterAfter.GetReliability() != characterBefore.GetReliability()+1 {
		t.Fatalf("Showcase photo reliability = %d, want %d", characterAfter.GetReliability(), characterBefore.GetReliability()+1)
	}
	totalAfter, err := opuser.FindUserTotalCount(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if totalAfter.GetPhotoShootCount() != totalBefore.GetPhotoShootCount()+1 {
		t.Fatalf("Showcase photo shoot count = %d, want %d", totalAfter.GetPhotoShootCount(), totalBefore.GetPhotoShootCount()+1)
	}
	userAfter, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(userAfter.GetNextPhotoImageIds()) != len(reservedBefore) ||
		slices.Contains(userAfter.GetNextPhotoImageIds(), usedAssetID) ||
		!allPhotoImageIDsUnique(userAfter.GetNextPhotoImageIds()) {
		t.Fatalf("Showcase.CreatePhoto did not rotate image IDs: before=%v after=%v", reservedBefore, userAfter.GetNextPhotoImageIds())
	}
}

func assertShowcaseCommonResponse(t *testing.T, api string, response *papi.CommonResponse) {
	t.Helper()
	if response == nil || response.GetUpdatedData() == nil || response.GetDeletedData() == nil {
		t.Fatalf("%s returned an incomplete common response: %v", api, response)
	}
}

func showcaseGRPCCallContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	return ctx, cancel
}
