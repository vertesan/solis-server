package test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

func TestShopScenario(t *testing.T) {
	setupTestUser(t)

	expectedShopIDs := map[string]struct{}{
		"shop_top_shop":                      {},
		"shop_top_dia":                       {},
		"shop_login_login-pack-01":           {},
		"shop_login_login-pack-02":           {},
		"shop_top_premium-shop-pack-22-0624": {},
	}

	homeClient := papi.NewHomeClient(testConfig.GrpcConn)
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	home, err := homeClient.Enter(ctx, &papi.HomeEnterRequest{})
	cancel()
	if err != nil {
		t.Fatalf("Home.Enter failed: %v", err)
	}
	assertShopIDSet(t, "Home.Enter", home.GetShopIds(), expectedShopIDs)

	client := papi.NewShopClient(testConfig.GrpcConn)
	ctx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	listed, err := client.List(ctx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("Shop.List failed: %v", err)
	}
	listedIDs := make([]string, 0, len(listed.GetShops()))
	listedByID := make(map[string]*papi.ShopInfo, len(listed.GetShops()))
	for _, shop := range listed.GetShops() {
		listedIDs = append(listedIDs, shop.GetId())
		listedByID[shop.GetId()] = shop
		items := listShopItems(t, client, shop.GetId()).GetShopItems()
		if len(items) == 0 {
			t.Fatalf("Shop.List exposed %q but Shop.ListItem returned no items", shop.GetId())
		}
		for _, item := range items {
			assertNoRealMoneyOrLimit(t, shop.GetId(), item)
		}
	}
	assertShopIDSet(t, "Shop.List", listedIDs, expectedShopIDs)

	catalog, err := opsolis.FindAllShopCatalogEntries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != len(expectedShopIDs) {
		t.Fatalf("stored shop catalog has %d entries, want %d", len(catalog), len(expectedShopIDs))
	}
	for _, entry := range catalog {
		if entry.Info == nil {
			t.Fatalf("stored shop catalog contains an item-only entry: %q", entry.ShopId)
		}
		if _, ok := expectedShopIDs[entry.ShopId]; !ok {
			t.Fatalf("stored shop catalog contains removed shop %q", entry.ShopId)
		}
		if len(entry.Items) == 0 {
			t.Fatalf("stored shop %q has no ListItem content", entry.ShopId)
		}
	}

	verifyFreeLoginPackages(t, homeClient, client, listedByID)

	verifyUnlimitedPremiumTicketPurchase(t, client)
	verifyFreeDiaPurchase(t, client)
	promoteTestUser(t)
	verifyInGameCurrencyPurchase(t, client)
}

func verifyFreeLoginPackages(
	t *testing.T,
	homeClient papi.HomeClient,
	shopClient papi.ShopClient,
	initialShopInfos map[string]*papi.ShopInfo,
) {
	t.Helper()
	shopIDs := []string{"shop_login_login-pack-01", "shop_login_login-pack-02"}
	expectedDailyFreeStone := int64(0)
	packageItemIDs := make(map[string]string, len(shopIDs))
	packageLengths := make(map[string]int32, len(shopIDs))

	for _, shopID := range shopIDs {
		info := initialShopInfos[shopID]
		if info == nil || info.GetIsPurchased() || info.GetProgress() != 0 || info.GetMaxProgress() <= 0 {
			t.Fatalf("new user's login package shop %q has invalid state: %v", shopID, info)
		}
		pack := getLoginBonusPackage(t, shopClient, shopID)
		if pack.GetIsPurchased() || pack.GetShopItem() == nil || len(pack.GetLoginBonuses()) == 0 {
			t.Fatalf("new user's login package %q is already active or incomplete: %v", shopID, pack)
		}
		assertNoRealMoneyOrLimit(t, shopID, pack.GetShopItem())
		for _, bonus := range pack.GetLoginBonuses() {
			if bonus.GetIsReceived() {
				t.Fatalf("unpurchased login package %q has a received day: %v", shopID, pack)
			}
		}
		dailyAmount, err := freeStoneAutoRewardAmount(pack.GetLoginBonuses()[0].GetRewardId())
		if err != nil {
			t.Fatalf("login package %q first reward is invalid: %v", shopID, err)
		}
		expectedDailyFreeStone += dailyAmount
		packageItemIDs[shopID] = pack.GetShopItem().GetId()
		packageLengths[shopID] = int32(len(pack.GetLoginBonuses()))

		for attempt := 1; attempt <= 2; attempt++ {
			ctx, cancel, err := getFullGrpcCtx()
			if err != nil {
				t.Fatal(err)
			}
			response, err := shopClient.Purchase(ctx, &papi.ShopPurchaseRequest{ShopItemId: pack.GetShopItem().GetId()})
			cancel()
			if err != nil {
				t.Fatalf("free login package %q activation attempt %d failed: %v", shopID, attempt, err)
			}
			if response.GetCommonResponse() == nil || len(response.GetRewards()) != 0 {
				t.Fatalf("free login package %q activation returned an immediate reward: %v", shopID, response)
			}
		}
		activated := getLoginBonusPackage(t, shopClient, shopID)
		if !activated.GetIsPurchased() {
			t.Fatalf("free login package %q was not activated", shopID)
		}
		for _, bonus := range activated.GetLoginBonuses() {
			if bonus.GetIsReceived() {
				t.Fatalf("login package %q granted a daily reward during activation: %v", shopID, activated)
			}
		}
	}

	state, err := opuser.FindUserShopPurchase(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, shopID := range shopIDs {
		if !containsShopString(state.PurchasedItemIds, packageItemIDs[shopID]) ||
			state.LoginBonusProgress[shopID] != 0 || state.LoginBonusLastGrantTimes[shopID] != 0 {
			t.Fatalf("login package %q activation state is invalid: %v", shopID, state)
		}
	}

	before, err := opuser.FindUserBalance(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	login, err := homeClient.Login(ctx, &papi.HomeLoginRequest{})
	cancel()
	if err != nil {
		t.Fatalf("Home.Login with active free packages failed: %v", err)
	}
	after, err := opuser.FindUserBalance(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if int64(after.GetFreeBalance()) != int64(before.GetFreeBalance())+expectedDailyFreeStone ||
		after.GetPaidBalance() != before.GetPaidBalance() {
		t.Fatalf("daily login package balance mismatch: before=%v after=%v expected free delta=%d", before, after, expectedDailyFreeStone)
	}
	updatedBalance := login.GetCommonResponse().GetUpdatedData().GetBalance()
	if updatedBalance == nil || updatedBalance.GetFreeBalance() != after.GetFreeBalance() ||
		updatedBalance.GetPaidBalance() != after.GetPaidBalance() {
		t.Fatalf("Home.Login omitted the daily package balance post-image: %v", login)
	}
	assertPurchasedLoginPackageInfos(t, login.GetPurchasedLoginBonusShopInfos(), shopIDs, packageLengths, 1)

	state, err = opuser.FindUserShopPurchase(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	firstGrantTimes := make(map[string]int64, len(shopIDs))
	for _, shopID := range shopIDs {
		if state.LoginBonusProgress[shopID] != 1 || state.LoginBonusLastGrantTimes[shopID] <= 0 {
			t.Fatalf("login package %q did not persist day-one grant state: %v", shopID, state)
		}
		firstGrantTimes[shopID] = state.LoginBonusLastGrantTimes[shopID]
		pack := getLoginBonusPackage(t, shopClient, shopID)
		if !pack.GetIsPurchased() || !loginBonusDayReceived(pack, 1) {
			t.Fatalf("login package %q endpoint omitted day-one progress: %v", shopID, pack)
		}
	}

	ctx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	secondLogin, err := homeClient.Login(ctx, &papi.HomeLoginRequest{})
	cancel()
	if err != nil {
		t.Fatalf("same-day Home.Login failed: %v", err)
	}
	secondBalance, err := opuser.FindUserBalance(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if secondBalance.GetFreeBalance() != after.GetFreeBalance() || secondBalance.GetPaidBalance() != after.GetPaidBalance() {
		t.Fatalf("same-day Home.Login granted package rewards twice: first=%v second=%v", after, secondBalance)
	}
	assertPurchasedLoginPackageInfos(t, secondLogin.GetPurchasedLoginBonusShopInfos(), shopIDs, packageLengths, 1)
	state, err = opuser.FindUserShopPurchase(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, shopID := range shopIDs {
		if state.LoginBonusProgress[shopID] != 1 || state.LoginBonusLastGrantTimes[shopID] != firstGrantTimes[shopID] {
			t.Fatalf("same-day login changed package %q grant state: %v", shopID, state)
		}
	}

	ctx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	listed, err := shopClient.List(ctx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("Shop.List after login package grant failed: %v", err)
	}
	for _, info := range listed.GetShops() {
		for _, shopID := range shopIDs {
			if info.GetId() == shopID && (!info.GetIsPurchased() || info.GetProgress() != 1 ||
				info.GetMaxProgress() != packageLengths[shopID]) {
				t.Fatalf("Shop.List returned stale login package state for %q: %v", shopID, info)
			}
		}
	}
}

func getLoginBonusPackage(t *testing.T, client papi.ShopClient, shopID string) *papi.LoginBonusPackageItem {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.GetLoginBonusPackageItem(ctx, &papi.ShopGetLoginBonusPackageItemRequest{ShopId: shopID})
	cancel()
	if err != nil {
		t.Fatalf("Shop.GetLoginBonusPackageItem(%q) failed: %v", shopID, err)
	}
	if response.GetCommonResponse() == nil || response.GetLoginBonusPackageItem() == nil {
		t.Fatalf("Shop.GetLoginBonusPackageItem(%q) returned incomplete data: %v", shopID, response)
	}
	return response.GetLoginBonusPackageItem()
}

func freeStoneAutoRewardAmount(rewardID string) (int64, error) {
	parts := strings.Split(rewardID, ";")
	if len(parts) != 4 || parts[0] != "rd_auto" || parts[1] != "stone" || parts[2] != "" {
		return 0, strconv.ErrSyntax
	}
	amount, err := strconv.ParseInt(parts[3], 10, 32)
	if err != nil || amount <= 0 {
		return 0, strconv.ErrSyntax
	}
	return amount, nil
}

func assertPurchasedLoginPackageInfos(
	t *testing.T,
	infos []*papi.PurchasedLoginBonusShopInfo,
	shopIDs []string,
	packageLengths map[string]int32,
	wantProgress int32,
) {
	t.Helper()
	if len(infos) != len(shopIDs) {
		t.Fatalf("Home.Login returned %d purchased login packages, want %d: %v", len(infos), len(shopIDs), infos)
	}
	seen := make(map[string]struct{}, len(infos))
	for _, purchased := range infos {
		info := purchased.GetShopInfo()
		pack := purchased.GetLoginBonusPackageItem()
		shopID := info.GetId()
		if _, duplicate := seen[shopID]; duplicate {
			t.Fatalf("Home.Login returned duplicate login package %q", shopID)
		}
		seen[shopID] = struct{}{}
		wantLength, expected := packageLengths[shopID]
		if !expected || !info.GetIsPurchased() || info.GetProgress() != wantProgress ||
			info.GetMaxProgress() != wantLength || pack == nil || !pack.GetIsPurchased() {
			t.Fatalf("Home.Login returned invalid login package state for %q: %v", shopID, purchased)
		}
		for _, bonus := range pack.GetLoginBonuses() {
			if bonus.GetIsReceived() != (bonus.GetDay() > 0 && bonus.GetDay() <= wantProgress) {
				t.Fatalf("Home.Login returned invalid received days for %q: %v", shopID, pack)
			}
		}
	}
}

func loginBonusDayReceived(pack *papi.LoginBonusPackageItem, day int32) bool {
	for _, bonus := range pack.GetLoginBonuses() {
		if bonus.GetDay() == day {
			return bonus.GetIsReceived()
		}
	}
	return false
}

func verifyUnlimitedPremiumTicketPurchase(t *testing.T, client papi.ShopClient) {
	t.Helper()
	const shopID = "shop_top_premium-shop-pack-22-0624"
	items := listShopItems(t, client, shopID).GetShopItems()
	if len(items) != 1 {
		t.Fatalf("premium ticket shop returned %d items, want 1", len(items))
	}
	item := items[0]
	reward, err := opmaster.FindReward(context.Background(), item.GetRewardId())
	if err != nil {
		t.Fatalf("failed to read premium shop reward %q: %v", item.GetRewardId(), err)
	}
	if len(reward.GetResources()) != 1 ||
		reward.GetResources()[0].GetResourceType() != penum.ResourceType_ResourceType_Item ||
		reward.GetResources()[0].GetResourceId() == "" || reward.GetResources()[0].GetAmount() <= 0 {
		t.Fatalf("premium shop reward is not one valid item resource: %v", reward)
	}
	resource := reward.GetResources()[0]
	before := userItemAmount(t, resource.GetResourceId())
	for purchaseNumber := int64(1); purchaseNumber <= 2; purchaseNumber++ {
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Purchase(ctx, &papi.ShopPurchaseRequest{ShopItemId: item.GetId()})
		cancel()
		if err != nil {
			t.Fatalf("unlimited premium ticket purchase %d failed: %v", purchaseNumber, err)
		}
		want := before + purchaseNumber*int64(resource.GetAmount())
		if got := userItemAmount(t, resource.GetResourceId()); got != want {
			t.Fatalf("premium ticket amount after purchase %d is %d, want %d", purchaseNumber, got, want)
		}
		if response.GetCommonResponse() == nil ||
			!containsUpdatedShopItem(response.GetCommonResponse().GetUpdatedData().GetItems(), resource.GetResourceId(), want) {
			t.Fatalf("premium purchase %d omitted the item post-image: %v", purchaseNumber, response)
		}
	}
	state, err := opuser.FindUserShopPurchase(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if state.PurchaseCounts[item.GetId()] != 0 || !containsShopString(state.PurchasedItemIds, item.GetId()) {
		t.Fatalf("unlimited premium purchase persisted invalid state: %v", state)
	}
}

func verifyFreeDiaPurchase(t *testing.T, client papi.ShopClient) {
	t.Helper()
	items := listShopItems(t, client, "shop_top_dia").GetShopItems()
	if len(items) == 0 {
		t.Fatal("dia shop returned no items")
	}
	item := items[0]
	before, err := opuser.FindUserBalance(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Purchase(ctx, &papi.ShopPurchaseRequest{ShopItemId: item.GetId()})
	cancel()
	if err != nil {
		t.Fatalf("free dia purchase failed: %v", err)
	}
	after, err := opuser.FindUserBalance(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	var granted int64
	for _, reward := range response.GetRewards() {
		if reward.GetResourceType() == penum.ResourceType_ResourceType_PaidStone {
			granted += reward.GetAmountI64()
		}
	}
	if granted <= 0 || int64(after.GetPaidBalance()) != int64(before.GetPaidBalance())+granted {
		t.Fatalf("free dia purchase balance mismatch: before=%v after=%v rewards=%v", before, after, response.GetRewards())
	}
}

func verifyInGameCurrencyPurchase(t *testing.T, client papi.ShopClient) {
	t.Helper()
	items := listShopItems(t, client, "shop_top_shop").GetShopItems()
	item, cost := findInGameCurrencyShopItem(items)
	if item == nil {
		t.Fatal("normal shop has no supported in-game-currency item")
	}
	before, err := opuser.FindUserBalance(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Purchase(ctx, &papi.ShopPurchaseRequest{ShopItemId: item.GetId()})
	cancel()
	if err != nil {
		t.Fatalf("in-game-currency Shop.Purchase failed: %v", err)
	}
	after, err := opuser.FindUserBalance(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	switch cost.GetResourceType() {
	case penum.ResourceType_ResourceType_Stone:
		if int64(after.GetFreeBalance()) != int64(before.GetFreeBalance())-int64(cost.GetAmount()) {
			t.Fatalf("free-stone cost mismatch: before=%v after=%v cost=%v", before, after, cost)
		}
	case penum.ResourceType_ResourceType_PaidStone:
		if int64(after.GetPaidBalance()) != int64(before.GetPaidBalance())-int64(cost.GetAmount()) {
			t.Fatalf("paid-stone cost mismatch: before=%v after=%v cost=%v", before, after, cost)
		}
	}
	if response.GetCommonResponse() == nil || response.GetCommonResponse().GetUpdatedData().GetBalance() == nil ||
		len(response.GetRewards()) == 0 {
		t.Fatalf("in-game-currency purchase returned incomplete state: %v", response)
	}
}

func findInGameCurrencyShopItem(items []*papi.ShopItem) (*papi.ShopItem, *pmaster.ConsumptionResource) {
	for _, item := range items {
		if !item.GetUnlocked() || item.GetProductId() != "" || item.GetConsumptionId() == "" || item.GetRewardId() == "" {
			continue
		}
		consumption, err := opmaster.FindConsumption(context.Background(), item.GetConsumptionId())
		if err != nil || len(consumption.GetResources()) != 1 {
			continue
		}
		cost := consumption.GetResources()[0]
		if cost.GetAmount() <= 0 || cost.GetResourceType() != penum.ResourceType_ResourceType_Stone &&
			cost.GetResourceType() != penum.ResourceType_ResourceType_PaidStone {
			continue
		}
		reward, err := opmaster.FindReward(context.Background(), item.GetRewardId())
		if err != nil || len(reward.GetResources()) == 0 {
			continue
		}
		changesBalance := false
		for _, resource := range reward.GetResources() {
			if resource.GetResourceType() == penum.ResourceType_ResourceType_Stone ||
				resource.GetResourceType() == penum.ResourceType_ResourceType_PaidStone {
				changesBalance = true
				break
			}
		}
		if !changesBalance {
			return item, cost
		}
	}
	return nil, nil
}

func assertShopIDSet(t *testing.T, source string, actual []string, expected map[string]struct{}) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("%s returned shop IDs %v, want exactly %v", source, actual, expected)
	}
	seen := make(map[string]struct{}, len(actual))
	for _, id := range actual {
		if _, ok := expected[id]; !ok {
			t.Fatalf("%s returned removed shop %q", source, id)
		}
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("%s returned duplicate shop %q", source, id)
		}
		seen[id] = struct{}{}
	}
}

func assertNoRealMoneyOrLimit(t *testing.T, shopID string, item *papi.ShopItem) {
	t.Helper()
	if item == nil {
		t.Fatalf("shop %q returned a nil item", shopID)
	}
	if item.GetProductId() != "" || item.GetPurchaseLimit() != 0 || item.GetLeftCount() != 0 ||
		item.GetResetTimingType() != penum.ResetTimingType_ResetTimingType_Never || item.GetNextResetTime() != 0 {
		t.Fatalf("shop %q item still has real-money or purchase-limit metadata: %v", shopID, item)
	}
}

func listShopItems(t *testing.T, client papi.ShopClient, shopID string) *papi.ShopListItemResponse {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.ListItem(ctx, &papi.ShopListItemRequest{ShopId: shopID})
	cancel()
	if err != nil {
		t.Fatalf("Shop.ListItem(%q) failed: %v", shopID, err)
	}
	if response.GetCommonResponse() == nil {
		t.Fatalf("Shop.ListItem(%q) returned no CommonResponse: %v", shopID, response)
	}
	return response
}

func containsUpdatedShopItem(items []*ptransaction.UserItem, itemID string, amount int64) bool {
	for _, item := range items {
		if item.GetItemId() == itemID && item.GetAmount() == amount && item.GetExpiredTime() == 0 {
			return true
		}
	}
	return false
}

func containsShopString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
