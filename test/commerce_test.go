package test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// TestFreshCommerceCatalogIdentity verifies that creation follows the current
// Solis catalogs instead of retaining identities from an older rotation.
func TestFreshCommerceCatalogIdentity(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()

	shops, err := opsolis.FindAllShopCatalogEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantShopIDs := make([]string, 0, len(shops))
	wantShowcaseShopIDs := make([]string, 0, len(shops))
	for _, shop := range shops {
		if shop.Info == nil {
			continue
		}
		switch shop.Info.GetType() {
		case penum.ShopType_ShopType_ShowcaseToyShop,
			penum.ShopType_ShopType_DefaultShowcaseToyShop,
			penum.ShopType_ShopType_OnlyShowcaseShop:
			wantShowcaseShopIDs = append(wantShowcaseShopIDs, shop.ShopId)
		default:
			wantShopIDs = append(wantShopIDs, shop.ShopId)
		}
	}
	userShops, err := opuser.FindUserShopIds(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	userShowcaseShops, err := opuser.FindUserShowcaseToyShopIds(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	assertStringRoster(t, "fresh user shops", userShops.ShopIds, wantShopIDs)
	assertStringRoster(t, "fresh user showcase shops", userShowcaseShops.ShopIds, wantShowcaseShopIDs)

	booths, err := opsolis.FindAllExchangeBooths(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userBooths, err := opuser.FindUserExchangeBooth(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	assertStringRoster(t, "fresh user exchange catalog", userBooths.BoothIds, exchangeBoothIds(booths))

	homeClient := papi.NewHomeClient(testConfig.GrpcConn)
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	home, err := homeClient.Enter(grpcCtx, &papi.HomeEnterRequest{})
	cancel()
	if err != nil {
		t.Fatalf("Home.Enter failed for fresh commerce identity: %v", err)
	}
	assertStringSubset(t, "Home.Enter shops", home.GetShopIds(), userShops.ShopIds)
	assertStringSubset(t, "Home.Enter showcase shops", home.GetShowcaseToyShopIds(), userShowcaseShops.ShopIds)
	assertStringSubset(t, "Home.Enter exchange booths", home.GetExchangeBoothIds(), userBooths.BoothIds)
}

func TestCommerceAPIs(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)

	divisionClient := papi.NewDivisionClient(testConfig.GrpcConn)
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	divisionResponse, err := divisionClient.List(ctx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("Division.List failed: %v", err)
	}
	if divisionResponse.GetCommonResponse() == nil {
		t.Fatalf("Division.List returned incomplete response: %v", divisionResponse)
	}
	storedDivisions, err := opsolis.FindAllDivisions(context.Background())
	if err != nil {
		t.Fatalf("failed to read current division data: %v", err)
	}
	assertStringRoster(t, "Division.List", divisionIds(divisionResponse.GetDivisions()), divisionIds(storedDivisions))
	assertProtoRoster(t, "Division.List", divisionResponse.GetDivisions(), storedDivisions, func(division *papi.DivisionInfo) string {
		return division.GetId()
	})

	shopClient := papi.NewShopClient(testConfig.GrpcConn)
	shopCatalog, err := opsolis.FindAllShopCatalogEntries(context.Background())
	if err != nil {
		t.Fatalf("failed to read current shop catalog: %v", err)
	}
	ctx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	shopResponse, err := shopClient.List(ctx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("Shop.List failed: %v", err)
	}
	if shopResponse.GetCommonResponse() == nil {
		t.Fatalf("Shop.List returned incomplete response: %v", shopResponse)
	}
	shopCatalogByID := make(map[string]*smodel.ShopCatalogEntry, len(shopCatalog))
	for _, entry := range shopCatalog {
		shopCatalogByID[entry.ShopId] = entry
	}
	for _, actual := range shopResponse.GetShops() {
		entry := shopCatalogByID[actual.GetId()]
		if entry == nil {
			t.Fatalf("Shop.List returned shop %q absent from current catalog", actual.GetId())
		}
		expected := proto.Clone(entry.Info).(*papi.ShopInfo)
		// Purchase and login-package progress are rendered from per-user state.
		expected.IsPurchased = actual.GetIsPurchased()
		expected.Progress = actual.GetProgress()
		expected.MaxProgress = actual.GetMaxProgress()
		if !proto.Equal(actual, expected) {
			t.Fatalf("Shop.List response is inconsistent with current catalog: shop=%v catalog=%v", actual, expected)
		}
	}

	t.Run("ListItem", func(t *testing.T) {
		entry := firstShopCatalogEntry(shopResponse.GetShops(), shopCatalogByID, func(entry *smodel.ShopCatalogEntry) bool {
			return len(entry.Items) > 0
		})
		visible := entry != nil
		if entry == nil {
			entry = firstCatalogEntry(shopCatalog, func(entry *smodel.ShopCatalogEntry) bool {
				return entry.Info != nil && len(entry.Items) > 0
			})
		}
		if entry == nil {
			t.Skip("current shop catalog has no item list")
		}
		grpcCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		shopItems, err := shopClient.ListItem(grpcCtx, &papi.ShopListItemRequest{ShopId: entry.ShopId})
		cancel()
		if !visible {
			if status.Code(err) != codes.PermissionDenied {
				t.Fatalf("Shop.ListItem for a currently hidden shop returned %v, want PermissionDenied", err)
			}
			return
		}
		if err != nil {
			t.Fatalf("Shop.ListItem failed: %v", err)
		}
		if shopItems.GetCommonResponse() == nil {
			t.Fatalf("Shop.ListItem returned incomplete response: %v", shopItems)
		}
		storedItems := make(map[string]*papi.ShopItem, len(entry.Items))
		for _, item := range entry.Items {
			storedItems[item.GetId()] = item
		}
		for _, actual := range shopItems.GetShopItems() {
			expected := storedItems[actual.GetId()]
			if expected == nil {
				t.Fatalf("Shop.ListItem returned item %q absent from current catalog", actual.GetId())
			}
			expected = proto.Clone(expected).(*papi.ShopItem)
			expected.Unlocked = actual.GetUnlocked()
			// Stock and reset windows are per-user runtime state, not stable
			// server-wide catalog metadata.
			expected.LeftCount = actual.GetLeftCount()
			expected.NextResetTime = actual.GetNextResetTime()
			if !proto.Equal(actual, expected) {
				t.Fatalf("Shop.ListItem response is inconsistent with current catalog: item=%v catalog=%v", actual, expected)
			}
		}
	})

	t.Run("LoginBonusPackage", func(t *testing.T) {
		entry := firstShopCatalogEntry(shopResponse.GetShops(), shopCatalogByID, func(entry *smodel.ShopCatalogEntry) bool {
			return entry.LoginBonusPackageItem != nil
		})
		visible := entry != nil
		if entry == nil {
			entry = firstCatalogEntry(shopCatalog, func(entry *smodel.ShopCatalogEntry) bool {
				return entry.Info != nil && entry.LoginBonusPackageItem != nil
			})
		}
		if entry == nil {
			t.Skip("current shop catalog has no login bonus package")
		}
		grpcCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		response, err := shopClient.GetLoginBonusPackageItem(grpcCtx, &papi.ShopGetLoginBonusPackageItemRequest{ShopId: entry.ShopId})
		cancel()
		if !visible {
			if status.Code(err) != codes.PermissionDenied {
				t.Fatalf("Shop.GetLoginBonusPackageItem for a currently hidden shop returned %v, want PermissionDenied", err)
			}
			return
		}
		if err != nil {
			t.Fatalf("Shop.GetLoginBonusPackageItem failed: %v", err)
		}
		if response.GetCommonResponse() == nil || response.GetLoginBonusPackageItem() == nil {
			t.Fatalf("Shop.GetLoginBonusPackageItem returned incomplete response: %v", response)
		}
		actual := response.GetLoginBonusPackageItem()
		expected := proto.Clone(entry.LoginBonusPackageItem).(*papi.LoginBonusPackageItem)
		if expected.ShopItem != nil {
			expected.ShopItem.Unlocked = actual.GetShopItem().GetUnlocked()
		}
		expected.IsPurchased = actual.GetIsPurchased()
		actualReceivedByDay := make(map[int32]bool, len(actual.GetLoginBonuses()))
		for _, bonus := range actual.GetLoginBonuses() {
			actualReceivedByDay[bonus.GetDay()] = bonus.GetIsReceived()
		}
		for _, bonus := range expected.GetLoginBonuses() {
			bonus.IsReceived = actualReceivedByDay[bonus.GetDay()]
		}
		if !proto.Equal(actual, expected) {
			t.Fatalf("Shop.GetLoginBonusPackageItem is inconsistent with current catalog: package=%v catalog=%v", actual, expected)
		}
	})

	exchangeClient := papi.NewExchangeClient(testConfig.GrpcConn)
	ctx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	exchangeList, err := exchangeClient.List(ctx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("Exchange.List failed: %v", err)
	}
	if exchangeList.GetCommonResponse() == nil {
		t.Fatalf("Exchange.List returned incomplete response: %v", exchangeList)
	}
	catalogBooths, err := opsolis.FindAllExchangeBooths(context.Background())
	if err != nil {
		t.Fatalf("failed to read exchange catalog: %v", err)
	}
	catalogByID := make(map[string]*papi.ExchangeBooth, len(catalogBooths))
	for _, booth := range catalogBooths {
		catalogByID[booth.GetId()] = booth
	}
	for _, actualBooth := range exchangeList.GetBooths() {
		definition := catalogByID[actualBooth.GetId()]
		if definition == nil {
			t.Fatalf("Exchange.List returned booth %q absent from current catalog", actualBooth.GetId())
		}
		expectedBooth := proto.Clone(definition).(*papi.ExchangeBooth)
		expectedBooth.Unlocked = actualBooth.GetUnlocked()
		expectedBooth.Exchanges = actualBooth.GetExchanges()
		if !proto.Equal(actualBooth, expectedBooth) {
			t.Fatalf("Exchange.List booth is inconsistent with current catalog: booth=%v catalog=%v", actualBooth, expectedBooth)
		}
		for _, actualItem := range actualBooth.GetExchanges() {
			definitionItem := findExchangeItemForTest(definition, actualItem.GetId())
			if definitionItem == nil {
				t.Fatalf("Exchange.List returned item %q absent from current catalog", actualItem.GetId())
			}
			expectedItem := proto.Clone(definitionItem).(*papi.ExchangeItem)
			expectedItem.Unlocked = actualItem.GetUnlocked()
			expectedItem.LeftCount = actualItem.GetLeftCount()
			if !proto.Equal(actualItem, expectedItem) {
				t.Fatalf("Exchange.List item is inconsistent with current catalog: item=%v catalog=%v", actualItem, expectedItem)
			}
		}
	}

	t.Run("LockedItem", func(t *testing.T) {
		candidate := findLockedItemExchangeCandidate(t, exchangeList.GetBooths())
		if candidate == nil {
			t.Skip("current exchange rotation has no visible locked item with an item cost")
		}
		before := userItemAmount(t, candidate.booth.GetRequiredResourceId())
		grpcCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		_, err = exchangeClient.Execute(grpcCtx, &papi.ExchangeExecuteRequest{ExchangeId: candidate.item.GetId(), Count: 1})
		cancel()
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("Exchange.Execute condition-locked item returned %v, want PermissionDenied", err)
		}
		if got := userItemAmount(t, candidate.booth.GetRequiredResourceId()); got != before {
			t.Fatalf("condition-rejected exchange changed resources: got %d, want %d", got, before)
		}
	})

	t.Run("ItemReward", func(t *testing.T) {
		candidate := findExchangeCandidate(t, exchangeList.GetBooths(), penum.ResourceType_ResourceType_Item, true)
		if candidate == nil {
			t.Skip("current exchange rotation has no executable limited item reward")
		}
		costID := candidate.booth.GetRequiredResourceId()
		rewardID := candidate.reward.GetResourceId()
		beforeCost := userItemAmount(t, costID)
		beforeReward := userItemAmount(t, rewardID)

		grpcCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		response, err := exchangeClient.Execute(grpcCtx, &papi.ExchangeExecuteRequest{ExchangeId: candidate.item.GetId(), Count: 1})
		cancel()
		if err != nil {
			t.Fatalf("Exchange.Execute failed: %v", err)
		}
		if response.GetCommonResponse() == nil || response.GetAfterExchangeItem() == nil {
			t.Fatalf("Exchange.Execute returned incomplete response: %v", response)
		}
		if got, want := userItemAmount(t, costID), beforeCost-candidate.item.GetRequiredResourceAmount(); got != want {
			t.Fatalf("Exchange.Execute cost mismatch: got %d, want %d", got, want)
		}
		if got, want := userItemAmount(t, rewardID), beforeReward+int64(candidate.reward.GetAmount()); got != want {
			t.Fatalf("Exchange.Execute reward mismatch: got %d, want %d", got, want)
		}
		if got, want := response.GetAfterExchangeItem().GetLeftCount(), candidate.item.GetLeftCount()-1; got != want {
			t.Fatalf("Exchange.Execute stock mismatch: got %d, want %d", got, want)
		}
		results := response.GetResults()
		if len(results) != 1 || results[0].GetResourceType() != candidate.reward.GetResourceType() ||
			results[0].GetResourceId() != rewardID || results[0].GetAmountI64() != int64(candidate.reward.GetAmount()) {
			t.Fatalf("Exchange.Execute returned incorrect reward result: %v", results)
		}

		beforeRejectedCost := userItemAmount(t, costID)
		beforeRejectedReward := userItemAmount(t, rewardID)
		overLimitCount := response.GetAfterExchangeItem().GetLeftCount() + 1
		grpcCtx, cancel, err = getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		_, err = exchangeClient.Execute(grpcCtx, &papi.ExchangeExecuteRequest{ExchangeId: candidate.item.GetId(), Count: overLimitCount})
		cancel()
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("Exchange.Execute over stock returned %v, want FailedPrecondition", err)
		}
		if got := userItemAmount(t, costID); got != beforeRejectedCost {
			t.Fatalf("rejected exchange changed cost item: got %d, want %d", got, beforeRejectedCost)
		}
		if got := userItemAmount(t, rewardID); got != beforeRejectedReward {
			t.Fatalf("rejected exchange changed reward item: got %d, want %d", got, beforeRejectedReward)
		}
	})

	t.Run("PhotoAllInOneReward", func(t *testing.T) {
		candidate := findExchangeCandidate(t, exchangeList.GetBooths(), penum.ResourceType_ResourceType_PhotoAllInOne, false)
		if candidate == nil {
			t.Skip("current exchange rotation has no executable photo-all-in-one reward")
		}
		masterPhoto, err := opmaster.FindPhotoAllInOne(context.Background(), candidate.reward.GetResourceId())
		if err != nil {
			t.Fatalf("failed to read photo-all-in-one master data: %v", err)
		}
		photosBefore, err := opuser.FindAllUserPhotos(context.Background(), testConfig.TestUserSuid)
		if err != nil {
			t.Fatalf("failed to read user photos before exchange: %v", err)
		}
		costID := candidate.booth.GetRequiredResourceId()
		costBefore := userItemAmount(t, costID)
		grpcCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		response, err := exchangeClient.Execute(grpcCtx, &papi.ExchangeExecuteRequest{ExchangeId: candidate.item.GetId(), Count: 1})
		cancel()
		if err != nil {
			t.Fatalf("Exchange.Execute photo-all-in-one failed: %v", err)
		}
		if got, want := userItemAmount(t, costID), costBefore-candidate.item.GetRequiredResourceAmount(); got != want {
			t.Fatalf("photo exchange cost mismatch: got %d, want %d", got, want)
		}
		photosAfter, err := opuser.FindAllUserPhotos(context.Background(), testConfig.TestUserSuid)
		if err != nil {
			t.Fatalf("failed to read user photos after exchange: %v", err)
		}
		amount := int(candidate.reward.GetAmount())
		if len(photosAfter) != len(photosBefore)+amount {
			t.Fatalf("photo exchange persisted %d photos, want %d", len(photosAfter)-len(photosBefore), amount)
		}
		updatedPhotos := response.GetCommonResponse().GetUpdatedData().GetPhotos()
		if len(updatedPhotos) != amount {
			t.Fatalf("photo exchange returned %d updated photos, want %d: %v", len(updatedPhotos), amount, response)
		}
		for _, grantedPhoto := range updatedPhotos {
			if grantedPhoto.GetPhotoId() == "" || grantedPhoto.GetAssetId() != masterPhoto.GetAssetId() ||
				grantedPhoto.GetName() != masterPhoto.GetName() || grantedPhoto.GetRarity() != masterPhoto.GetRarity() ||
				grantedPhoto.GetLevel() != masterPhoto.GetLevel() || len(grantedPhoto.GetAbilities()) != len(masterPhoto.GetAbilities()) {
				t.Fatalf("photo exchange did not materialize master data: photo=%v master=%v", grantedPhoto, masterPhoto)
			}
			storedPhoto, err := opuser.FindUserPhoto(context.Background(), testConfig.TestUserSuid, grantedPhoto.GetPhotoId())
			if err != nil || storedPhoto.GetAssetId() != masterPhoto.GetAssetId() {
				t.Fatalf("photo exchange did not persist returned photo: photo=%v err=%v", storedPhoto, err)
			}
		}
		results := response.GetResults()
		if len(results) != 1 || results[0].GetResourceType() != candidate.reward.GetResourceType() ||
			results[0].GetResourceId() != candidate.reward.GetResourceId() || results[0].GetAmountI64() != int64(candidate.reward.GetAmount()) {
			t.Fatalf("photo exchange returned incorrect reward result: %v", results)
		}
	})
}

type exchangeCandidate struct {
	booth  *papi.ExchangeBooth
	item   *papi.ExchangeItem
	reward *pmaster.RewardResource
}

func firstShopCatalogEntry(
	shops []*papi.ShopInfo,
	catalog map[string]*smodel.ShopCatalogEntry,
	match func(*smodel.ShopCatalogEntry) bool,
) *smodel.ShopCatalogEntry {
	for _, shop := range shops {
		entry := catalog[shop.GetId()]
		if entry != nil && match(entry) {
			return entry
		}
	}
	return nil
}

func firstCatalogEntry(
	catalog []*smodel.ShopCatalogEntry,
	match func(*smodel.ShopCatalogEntry) bool,
) *smodel.ShopCatalogEntry {
	for _, entry := range catalog {
		if match(entry) {
			return entry
		}
	}
	return nil
}

func findLockedItemExchangeCandidate(t *testing.T, booths []*papi.ExchangeBooth) *exchangeCandidate {
	t.Helper()
	for _, booth := range booths {
		if booth.GetRequiredResourceType() != penum.ResourceType_ResourceType_Item || booth.GetRequiredResourceId() == "" {
			continue
		}
		for _, item := range booth.GetExchanges() {
			if item.GetRequiredResourceAmount() > 0 && (!booth.GetUnlocked() || !item.GetUnlocked()) {
				return &exchangeCandidate{booth: booth, item: item}
			}
		}
	}
	return nil
}

func findExchangeCandidate(
	t *testing.T,
	booths []*papi.ExchangeBooth,
	rewardType penum.ResourceType,
	requireLimited bool,
) *exchangeCandidate {
	t.Helper()
	for _, booth := range booths {
		if !booth.GetUnlocked() || booth.GetRequiredResourceType() != penum.ResourceType_ResourceType_Item || booth.GetRequiredResourceId() == "" {
			continue
		}
		for _, item := range booth.GetExchanges() {
			if !item.GetUnlocked() || item.GetRequiredResourceAmount() <= 0 {
				continue
			}
			if requireLimited && (item.GetExchangeLimit() <= 0 || item.GetLeftCount() <= 0) {
				continue
			}
			rewards, err := resolveCatalogRewards(context.Background(), item.GetRewardId())
			if err != nil || len(rewards) != 1 || rewards[0].GetResourceType() != rewardType || rewards[0].GetAmount() <= 0 {
				continue
			}
			if rewardType == penum.ResourceType_ResourceType_Item && rewards[0].GetResourceId() == booth.GetRequiredResourceId() {
				continue
			}
			if userItemAmount(t, booth.GetRequiredResourceId()) < item.GetRequiredResourceAmount() {
				continue
			}
			return &exchangeCandidate{booth: booth, item: item, reward: rewards[0]}
		}
	}
	return nil
}

func resolveCatalogRewards(ctx context.Context, rewardID string) ([]*pmaster.RewardResource, error) {
	if !strings.HasPrefix(rewardID, "rd_auto;") {
		reward, err := opmaster.FindReward(ctx, rewardID)
		if err != nil {
			return nil, err
		}
		return reward.GetResources(), nil
	}
	parts := strings.Split(rewardID, ";")
	if len(parts) != 4 {
		return nil, fmt.Errorf("malformed automatic reward %q", rewardID)
	}
	var resourceType penum.ResourceType
	switch parts[1] {
	case "item":
		resourceType = penum.ResourceType_ResourceType_Item
	case "photo_all_in_one":
		resourceType = penum.ResourceType_ResourceType_PhotoAllInOne
	default:
		return nil, fmt.Errorf("unsupported automatic reward type %q", parts[1])
	}
	amount, err := strconv.ParseInt(parts[3], 10, 32)
	if err != nil || amount <= 0 {
		return nil, fmt.Errorf("malformed automatic reward amount in %q", rewardID)
	}
	return []*pmaster.RewardResource{{ResourceType: resourceType, ResourceId: parts[2], Amount: int32(amount)}}, nil
}

func findExchangeItemForTest(booth *papi.ExchangeBooth, exchangeID string) *papi.ExchangeItem {
	for _, item := range booth.GetExchanges() {
		if item.GetId() == exchangeID {
			return item
		}
	}
	return nil
}

func divisionIds(divisions []*papi.DivisionInfo) []string {
	ids := make([]string, 0, len(divisions))
	for _, division := range divisions {
		ids = append(ids, division.GetId())
	}
	return ids
}

func exchangeBoothIds(booths []*papi.ExchangeBooth) []string {
	ids := make([]string, 0, len(booths))
	for _, booth := range booths {
		ids = append(ids, booth.GetId())
	}
	return ids
}
func assertStringRoster(t *testing.T, name string, got, want []string) {
	t.Helper()
	wantSet := make(map[string]struct{}, len(want))
	for _, id := range want {
		wantSet[id] = struct{}{}
	}
	for _, id := range got {
		if _, ok := wantSet[id]; !ok {
			t.Fatalf("%s returned unexpected id %q", name, id)
		}
		delete(wantSet, id)
	}
	if len(got) != len(want) || len(wantSet) != 0 {
		t.Fatalf("%s roster mismatch: got %d entries, want %d; missing %v", name, len(got), len(want), wantSet)
	}
}

func assertStringSubset(t *testing.T, name string, got, allowed []string) {
	t.Helper()
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, id := range allowed {
		allowedSet[id] = struct{}{}
	}
	for _, id := range got {
		if _, ok := allowedSet[id]; !ok {
			t.Fatalf("%s returned id %q outside the user's current catalog", name, id)
		}
	}
}

func assertProtoRoster[T proto.Message](t *testing.T, name string, got, want []T, id func(T) string) {
	t.Helper()
	wantByID := make(map[string]T, len(want))
	for _, value := range want {
		wantByID[id(value)] = value
	}
	for _, value := range got {
		expected, ok := wantByID[id(value)]
		if !ok || !proto.Equal(value, expected) {
			t.Fatalf("%s response does not match current database value for %q: got=%v want=%v", name, id(value), value, expected)
		}
	}
}

func userItemAmount(t *testing.T, itemId string) int64 {
	t.Helper()
	items, err := opuser.FindUserItems(context.Background(), testConfig.TestUserSuid, itemId)
	if err != nil {
		t.Fatalf("failed to read user item %q: %v", itemId, err)
	}
	var total int64
	for _, item := range items {
		total += item.GetAmount()
	}
	return total
}
