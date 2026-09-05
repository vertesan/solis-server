package test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestItemSellScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewItemClient(testConfig.GrpcConn)
	const itemID = "item-card_skill"
	master, err := opmaster.FindItem(ctx, itemID)
	if err != nil || master.GetSalePrice() <= 1 {
		t.Fatalf("missing saleable skill powder master: %v, %v", master, err)
	}
	price := int64(master.SalePrice)
	goldBefore, earnedBefore := saleGoldState(t)
	expired := time.Now().Add(-time.Hour).UnixMilli()
	soon := time.Now().Add(time.Hour).UnixMilli()
	later := time.Now().Add(2 * time.Hour).UnixMilli()
	for _, item := range []*ptransaction.UserItem{
		{ItemId: itemID, Amount: 20},
		{ItemId: itemID, Amount: 99, ExpiredTime: expired},
		{ItemId: itemID, Amount: 3, ExpiredTime: soon},
		{ItemId: itemID, Amount: 4, ExpiredTime: later},
	} {
		if err := opuser.UpsertUserItem(ctx, testConfig.TestUserSuid, item); err != nil {
			t.Fatal(err)
		}
	}
	response := saleCall(t, func(ctx context.Context) (*papi.ItemSellResponse, error) {
		return client.Sell(ctx, &papi.ItemSellRequest{ItemId: itemID, Amount: 5})
	})
	updated := response.GetCommonResponse().GetUpdatedData()
	if len(updated.GetItems()) != 2 || updated.Items[0].ExpiredTime != soon || updated.Items[0].Amount != 0 ||
		updated.Items[1].ExpiredTime != later || updated.Items[1].Amount != 2 {
		t.Fatalf("item sale must consume nearest expiry first: %v", updated.GetItems())
	}
	assertSaleGold(t, updated, goldBefore+5*price, earnedBefore+5*price)
	assertSaleItemStacks(t, itemID, map[int64]int64{0: 20, expired: 99, soon: 0, later: 2})

	for _, request := range []*papi.ItemSellRequest{
		{ItemId: itemID, Amount: 23}, // Expired stacks cannot fund the sale.
		{ItemId: itemID, Amount: 0},
		{ItemId: itemID, Amount: -1},
		{ItemId: itemID, Amount: math.MaxInt64},
	} {
		grpcCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Sell(grpcCtx, request)
		cancel()
		want := codes.InvalidArgument
		if request.Amount == 23 {
			want = codes.ResourceExhausted
		}
		if status.Code(err) != want {
			t.Fatalf("rejected Item.Sell %v: got %v, want %v", request, err, want)
		}
	}
	assertSaleGold(t, nil, goldBefore+5*price, earnedBefore+5*price)
	assertSaleItemStacks(t, itemID, map[int64]int64{0: 20, expired: 99, soon: 0, later: 2})

	// An item with no master sale price must not be consumed.
	const unsaleableID = "item-accessory_limit_break-01"
	unsaleable, err := opmaster.FindItem(ctx, unsaleableID)
	if err != nil || unsaleable.GetSalePrice() != 0 {
		t.Fatalf("missing unsaleable master fixture: %v, %v", unsaleable, err)
	}
	if err := opuser.UpsertUserItem(ctx, testConfig.TestUserSuid, &ptransaction.UserItem{ItemId: unsaleableID, Amount: 1}); err != nil {
		t.Fatal(err)
	}
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Sell(grpcCtx, &papi.ItemSellRequest{ItemId: unsaleableID, Amount: 1})
	cancel()
	if status.Code(err) != codes.FailedPrecondition || userItemAmount(t, unsaleableID) != 1 {
		t.Fatalf("unsaleable item was accepted or consumed: %v", err)
	}

	// Selling the remaining usable balance leaves zero post-images for the client.
	response = saleCall(t, func(ctx context.Context) (*papi.ItemSellResponse, error) {
		return client.Sell(ctx, &papi.ItemSellRequest{ItemId: itemID, Amount: 22})
	})
	assertSaleGold(t, response.GetCommonResponse().GetUpdatedData(), goldBefore+27*price, earnedBefore+27*price)
	assertSaleItemStacks(t, itemID, map[int64]int64{0: 0, expired: 99, soon: 0, later: 0})

	// A valid sale can exceed the int32 master reward format; Gold stays int64.
	largeAmount := int64(math.MaxInt32)/price + 1
	if err := opuser.UpsertUserItem(ctx, testConfig.TestUserSuid, &ptransaction.UserItem{ItemId: itemID, Amount: largeAmount}); err != nil {
		t.Fatal(err)
	}
	response = saleCall(t, func(ctx context.Context) (*papi.ItemSellResponse, error) {
		return client.Sell(ctx, &papi.ItemSellRequest{ItemId: itemID, Amount: largeAmount})
	})
	assertSaleGold(t, response.GetCommonResponse().GetUpdatedData(), goldBefore+(27+largeAmount)*price, earnedBefore+(27+largeAmount)*price)
}

func TestAccessorySellScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewAccessoryClient(testConfig.GrpcConn)
	const accessoryID = "ac-1-visual-07"
	master, err := opmaster.FindAccessory(ctx, accessoryID)
	if err != nil || master.GetSalePrice() <= 0 {
		t.Fatalf("missing saleable accessory master: %v, %v", master, err)
	}
	price := int64(master.SalePrice)
	goldBefore, earnedBefore := saleGoldState(t)
	if err := opuser.UpsertUserAccessory(ctx, testConfig.TestUserSuid, &ptransaction.UserAccessory{AccessoryId: accessoryID, Amount: 7}); err != nil {
		t.Fatal(err)
	}
	positions, err := opuser.FindAllUserDeckPositions(ctx, testConfig.TestUserSuid)
	if err != nil || len(positions) < 2 {
		t.Fatalf("missing initial deck positions: %v", err)
	}
	// Two positions in the same deck need two owned copies.
	first, second := positions[0], positions[1]
	if first.Number != second.Number {
		t.Fatal("initial deck positions do not belong to the same deck")
	}
	first.Part1AccessoryId, second.Part1AccessoryId = accessoryID, accessoryID
	if err := opuser.UpsertManyUserDeckPositions(ctx, testConfig.TestUserSuid, []*ptransaction.UserDeckPosition{first, second}); err != nil {
		t.Fatal(err)
	}
	assertAccessorySellRejected(t, client, accessoryID, 6, codes.FailedPrecondition)
	assertAccessorySellRejected(t, client, accessoryID, 8, codes.ResourceExhausted)
	assertAccessorySellRejected(t, client, accessoryID, 0, codes.InvalidArgument)
	assertSaleAccessoryAmount(t, accessoryID, 7)
	assertSaleGold(t, nil, goldBefore, earnedBefore)
	response := saleCall(t, func(ctx context.Context) (*papi.AccessorySellResponse, error) {
		return client.Sell(ctx, &papi.AccessorySellRequest{AccessoryId: accessoryID, Count: 5})
	})
	updated := response.GetCommonResponse().GetUpdatedData()
	if len(updated.GetAccessories()) != 1 || updated.Accessories[0].Amount != 2 || len(updated.GetDeckPositions()) != 0 {
		t.Fatalf("accessory sale returned wrong ownership/deck changes: %v", updated)
	}
	assertSaleGold(t, updated, goldBefore+5*price, earnedBefore+5*price)
	assertAccessorySellRejected(t, client, accessoryID, 1, codes.FailedPrecondition)

	// League equipment must remain owned after normal equipment is removed.
	first.Part1AccessoryId, second.Part1AccessoryId = "", ""
	if err := opuser.UpsertManyUserDeckPositions(ctx, testConfig.TestUserSuid, []*ptransaction.UserDeckPosition{first, second}); err != nil {
		t.Fatal(err)
	}
	league := &ptransaction.UserLeagueDeckPosition{LeagueSeasonId: "test-inventory-sale", DeckType: 1, Position: 1, CardId: first.CardId, Part1AccessoryId: accessoryID}
	if err := opuser.UpsertUserLeagueDeckPosition(ctx, testConfig.TestUserSuid, league); err != nil {
		t.Fatal(err)
	}
	assertAccessorySellRejected(t, client, accessoryID, 2, codes.FailedPrecondition)
	response = saleCall(t, func(ctx context.Context) (*papi.AccessorySellResponse, error) {
		return client.Sell(ctx, &papi.AccessorySellRequest{AccessoryId: accessoryID, Count: 1})
	})
	assertSaleAccessoryAmount(t, accessoryID, 1)
	assertSaleGold(t, response.GetCommonResponse().GetUpdatedData(), goldBefore+6*price, earnedBefore+6*price)
	league.Part1AccessoryId = ""
	if err := opuser.UpsertUserLeagueDeckPosition(ctx, testConfig.TestUserSuid, league); err != nil {
		t.Fatal(err)
	}
	response = saleCall(t, func(ctx context.Context) (*papi.AccessorySellResponse, error) {
		return client.Sell(ctx, &papi.AccessorySellRequest{AccessoryId: accessoryID, Count: 1})
	})
	assertSaleAccessoryAmount(t, accessoryID, 0)
	assertSaleGold(t, response.GetCommonResponse().GetUpdatedData(), goldBefore+7*price, earnedBefore+7*price)
	assertAccessorySellRejected(t, client, accessoryID, 1, codes.ResourceExhausted)

	// Enhancement must use the same equipped-quantity protection as sales.
	settingID, err := config.GetGameSettingId()
	if err != nil {
		t.Fatal(err)
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		t.Fatal(err)
	}
	const targetID = "ac-1-visual-08"
	target, err := opmaster.FindAccessory(ctx, targetID)
	if err != nil {
		t.Fatal(err)
	}
	material, err := opmaster.FindAccessoryEnhancementMaterial(ctx, target)
	if err != nil || material.GetId() != accessoryID {
		t.Fatalf("unexpected enhancement predecessor: %v, %v", material, err)
	}
	amount := int64(setting.AccessoryEnhancesRequiredMaterialAmount) + 1
	if err := opuser.UpsertUserAccessory(ctx, testConfig.TestUserSuid, &ptransaction.UserAccessory{AccessoryId: accessoryID, Amount: amount}); err != nil {
		t.Fatal(err)
	}
	first.Part1AccessoryId, second.Part1AccessoryId = accessoryID, accessoryID
	if err := opuser.UpsertManyUserDeckPositions(ctx, testConfig.TestUserSuid, []*ptransaction.UserDeckPosition{first, second}); err != nil {
		t.Fatal(err)
	}
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Enhance(grpcCtx, &papi.AccessoryEnhanceRequest{AccessoryId: targetID, Count: 1, MaterialType: penum.AccessoryEnhanceMaterialType_AccessoryEnhanceMaterialType_ExcludeAllDeck})
	cancel()
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("enhancement consumed equipped accessory copies: %v", err)
	}
	assertSaleAccessoryAmount(t, accessoryID, amount)
	assertSaleGold(t, nil, goldBefore+7*price, earnedBefore+7*price)

	// Opting in to consuming equipped copies must clear only excess slots.
	enhancement, err := opmaster.FindAccessoryEnhancement(ctx, target.Rarity)
	if err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserPoint(ctx, testConfig.TestUserSuid, &ptransaction.UserPoint{
		PointType: penum.PointType_PointType_Gold,
		Amount:    enhancement.RequiredGold,
	}); err != nil {
		t.Fatal(err)
	}
	enhanced := saleCall(t, func(ctx context.Context) (*papi.AccessoryEnhanceResponse, error) {
		return client.Enhance(ctx, &papi.AccessoryEnhanceRequest{AccessoryId: targetID, Count: 1, MaterialType: penum.AccessoryEnhanceMaterialType_AccessoryEnhanceMaterialType_AllUserAccessory})
	})
	assertSaleAccessoryAmount(t, accessoryID, 1)
	assertSaleAccessoryAmount(t, targetID, 1)
	if len(enhanced.GetCommonResponse().GetUpdatedData().GetDeckPositions()) != 1 {
		t.Fatalf("enhancement did not clear the excess equipped copy: %v", enhanced)
	}
	positions, err = opuser.FindUserDeckPositions(ctx, testConfig.TestUserSuid, first.Number)
	if err != nil {
		t.Fatal(err)
	}
	var equipped int
	for _, position := range positions {
		if position.Part1AccessoryId == accessoryID {
			equipped++
		}
		if position.Part2AccessoryId == accessoryID {
			equipped++
		}
	}
	if equipped != 1 {
		t.Fatalf("enhancement left %d equipped copies, want 1", equipped)
	}
}

func saleCall[T any](t *testing.T, call func(context.Context) (T, error)) T {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	result, err := call(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func saleGoldState(t *testing.T) (int64, int64) {
	t.Helper()
	point, err := opuser.FindUserPoint(context.Background(), testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil {
		t.Fatal(err)
	}
	total, err := opuser.FindUserTotalCount(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	return point.Amount, total.PointGoldAmount
}

func assertSaleGold(t *testing.T, updated *papi.UpdatedData, amount, earned int64) {
	t.Helper()
	if updated != nil && (findPoint(updated.GetPoints(), penum.PointType_PointType_Gold).GetAmount() != amount || updated.GetTotalCount().GetPointGoldAmount() != earned) {
		t.Fatalf("sale response Gold is inconsistent: points=%v, total=%v, want=%d/%d", updated.GetPoints(), updated.GetTotalCount(), amount, earned)
	}
	if gotAmount, gotEarned := saleGoldState(t); gotAmount != amount || gotEarned != earned {
		t.Fatalf("persisted sale Gold=%d/%d, want %d/%d", gotAmount, gotEarned, amount, earned)
	}
}

func assertSaleItemStacks(t *testing.T, id string, want map[int64]int64) {
	t.Helper()
	items, err := opuser.FindUserItems(context.Background(), testConfig.TestUserSuid, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(want) {
		t.Fatalf("persisted item stacks=%v, want %v", items, want)
	}
	for _, item := range items {
		amount, ok := want[item.ExpiredTime]
		if !ok || item.Amount != amount {
			t.Fatalf("persisted item stack=%v, want %v", item, want)
		}
	}
}

func assertSaleAccessoryAmount(t *testing.T, id string, amount int64) {
	t.Helper()
	accessory, err := opuser.FindUserAccessory(context.Background(), testConfig.TestUserSuid, id)
	if err != nil || accessory.GetAmount() != amount {
		t.Fatalf("persisted accessory=%v, want amount=%d: %v", accessory, amount, err)
	}
}

func assertAccessorySellRejected(t *testing.T, client papi.AccessoryClient, id string, count int32, want codes.Code) {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	_, err = client.Sell(ctx, &papi.AccessorySellRequest{AccessoryId: id, Count: count})
	if status.Code(err) != want {
		t.Fatalf("Accessory.Sell count=%d returned %v, want %v", count, err, want)
	}
}
