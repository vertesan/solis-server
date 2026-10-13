package test

import (
	"context"
	"reflect"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// TestDiamondGachaSpendingScenario exercises the real Draw transaction, including
// the mixed-wallet response and unsuccessful requests not advancing the gacha.
func TestDiamondGachaSpendingScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	client := papi.NewGachaClient(testConfig.GrpcConn)
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := client.List(ctx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	type spendingCase struct {
		name                           string
		free, paid, wantFree, wantPaid int32
		failure                        bool
	}
	for _, resourceType := range []penum.ResourceType{penum.ResourceType_ResourceType_Stone, penum.ResourceType_ResourceType_PaidStone} {
		t.Run(resourceType.String(), func(t *testing.T) {
			info, button, cost := diamondGachaButton(t, catalog.GetGachas(), resourceType)
			cases := []spendingCase{
				{name: "insufficient combined", free: cost / 2, paid: cost - cost/2 - 1, wantFree: cost / 2, wantPaid: cost - cost/2 - 1, failure: true},
			}
			if resourceType == penum.ResourceType_ResourceType_Stone {
				cases = append(cases,
					spendingCase{name: "red first", free: cost + 10, paid: cost + 20, wantFree: 10, wantPaid: cost + 20},
					spendingCase{name: "mixed wallets", free: cost / 2, paid: cost, wantFree: 0, wantPaid: cost / 2},
					spendingCase{name: "blue alone", free: 0, paid: cost + 10, wantFree: 0, wantPaid: 10},
				)
			} else {
				cases = append(cases,
					spendingCase{name: "red cannot pay blue cost", free: cost * 2, paid: cost - 1, wantFree: cost * 2, wantPaid: cost - 1, failure: true},
					spendingCase{name: "blue only cost", free: cost * 2, paid: cost + 10, wantFree: cost * 2, wantPaid: 10},
				)
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					setDiamondTestBalance(t, tc.free, tc.paid)
					before, err := opuser.FindUserGachaInfo(context.Background(), testConfig.TestUserSuid, info.GetId())
					if err != nil {
						t.Fatal(err)
					}
					ctx, cancel, err := getFullGrpcCtx()
					if err != nil {
						t.Fatal(err)
					}
					response, err := client.Draw(ctx, &papi.GachaDrawRequest{GachaButtonId: button.GetId(), ExecCount: 1})
					cancel()
					if tc.failure {
						if status.Code(err) != codes.ResourceExhausted {
							t.Fatalf("Draw error = %v, want ResourceExhausted", err)
						}
						after, err := opuser.FindUserGachaInfo(context.Background(), testConfig.TestUserSuid, info.GetId())
						if err != nil || !proto.Equal(before, after) {
							t.Fatalf("failed Draw changed gacha progress: err=%v before=%v after=%v", err, before, after)
						}
					} else {
						if err != nil {
							t.Fatal(err)
						}
						if len(response.GetRewards()) == 0 {
							t.Fatal("successful Draw returned no rewards")
						}
						want := make([]*ptransaction.ConsumptionResult, 0, 2)
						if tc.free != tc.wantFree {
							want = append(want, &ptransaction.ConsumptionResult{ResourceType: penum.ResourceType_ResourceType_Stone, Amount: int64(tc.free - tc.wantFree), BeforeAmount: int64(tc.free), AfterAmount: int64(tc.wantFree)})
						}
						if tc.paid != tc.wantPaid {
							want = append(want, &ptransaction.ConsumptionResult{ResourceType: penum.ResourceType_ResourceType_PaidStone, Amount: int64(tc.paid - tc.wantPaid), BeforeAmount: int64(tc.paid), AfterAmount: int64(tc.wantPaid)})
						}
						if len(response.GetConsumptionResults()) != len(want) {
							t.Fatalf("Draw debits = %v, want %v", response.GetConsumptionResults(), want)
						}
						for i := range want {
							if !proto.Equal(response.GetConsumptionResults()[i], want[i]) {
								t.Fatalf("Draw debit = %v, want %v", response.GetConsumptionResults()[i], want[i])
							}
						}
						assertDiamondResponseBalance(t, response.GetCommonResponse().GetUpdatedData().GetBalance(), tc.wantFree, tc.wantPaid)
					}
					assertDiamondStoredBalance(t, tc.wantFree, tc.wantPaid)
				})
			}
		})
	}
}

func diamondGachaButton(t *testing.T, infos []*papi.GachaInfo, resourceType penum.ResourceType) (*papi.GachaInfo, *pmaster.GachaButton, int32) {
	t.Helper()
	for _, info := range infos {
		if !info.GetUnlocked() {
			continue
		}
		master, err := opmaster.FindGacha(context.Background(), info.GetId())
		if err != nil || master.GetGachaType() != penum.GachaType_GachaType_Normal {
			continue
		}
		for _, state := range info.GetButtons() {
			button, err := opmaster.FindGachaButton(context.Background(), state.GetId())
			if err != nil || button.GetDrawCount() <= 0 || button.GetDrawCount() > 10 || button.GetConsumptionId() == "" {
				continue
			}
			if resourceType == penum.ResourceType_ResourceType_Stone && (button.GetLimitCount() > 0 || button.GetDiscountCount() > 0) {
				continue // This button must support all three consecutive successful requests.
			}
			consumptionID := button.GetConsumptionId()
			if button.GetDiscountConsumptionId() != "" && button.GetDiscountCount() > 0 {
				consumptionID = button.GetDiscountConsumptionId()
			}
			consumption, err := opmaster.FindConsumption(context.Background(), consumptionID)
			if err != nil || len(consumption.GetResources()) != 1 {
				continue
			}
			resource := consumption.GetResources()[0]
			if resource.GetResourceType() == resourceType && resource.GetAmount() >= 2 && resource.GetAmount()%2 == 0 {
				return info, button, resource.GetAmount()
			}
		}
	}
	t.Skipf("current gacha catalog has no visible normal button accepting %v", resourceType)
	return nil, nil, 0
}

// TestDiamondShopSpendingScenario covers the user's exact 2700/5000 examples
// through Shop.Purchase and rolls back a debit when later reward granting fails.
func TestDiamondShopSpendingScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	entry, err := opsolis.FindShopCatalogEntry(ctx, "shop_top_shop")
	if err != nil {
		t.Fatal(err)
	}
	original := *entry
	t.Cleanup(func() {
		if err := opsolis.UpsertShopCatalogEntry(context.Background(), &original); err != nil {
			t.Errorf("restore shop catalog: %v", err)
		}
	})
	entry.Items = append([]*papi.ShopItem(nil), entry.Items...)
	entry.Items = append(entry.Items,
		&papi.ShopItem{Id: "test-diamond-red", ConsumptionId: "con-stone-2700", RewardId: "rd_auto;gold;;1"},
		&papi.ShopItem{Id: "test-diamond-blue", ConsumptionId: "con-paid_stone-5000", RewardId: "rd_auto;gold;;1"},
		&papi.ShopItem{Id: "test-diamond-rollback", ConsumptionId: "con-stone-2700", RewardId: "rd_auto;item;test-nonexistent-diamond-reward;1"},
	)
	if err := opsolis.UpsertShopCatalogEntry(ctx, entry); err != nil {
		t.Fatal(err)
	}
	client := papi.NewShopClient(testConfig.GrpcConn)
	for _, tc := range []struct {
		name, item                     string
		free, paid, wantFree, wantPaid int32
		failure                        bool
	}{
		{name: "insufficient combined", item: "test-diamond-red", free: 1000, paid: 1699, wantFree: 1000, wantPaid: 1699, failure: true},
		{name: "2700 mixed wallets", item: "test-diamond-red", free: 1000, paid: 2000, wantFree: 0, wantPaid: 300},
		{name: "red first", item: "test-diamond-red", free: 3000, paid: 2000, wantFree: 300, wantPaid: 2000},
		{name: "blue alone", item: "test-diamond-red", free: 0, paid: 3000, wantFree: 0, wantPaid: 300},
		{name: "red cannot pay 5000 blue", item: "test-diamond-blue", free: 10000, paid: 4999, wantFree: 10000, wantPaid: 4999, failure: true},
		{name: "5000 blue only", item: "test-diamond-blue", free: 10000, paid: 6000, wantFree: 10000, wantPaid: 1000},
		{name: "reward failure rolls back both wallets", item: "test-diamond-rollback", free: 1000, paid: 2000, wantFree: 1000, wantPaid: 2000, failure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setDiamondTestBalance(t, tc.free, tc.paid)
			purchaseBefore, beforeErr := opuser.FindUserShopPurchase(ctx, testConfig.TestUserSuid)
			goldBefore, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, penum.PointType_PointType_Gold)
			if err != nil {
				t.Fatal(err)
			}
			grpcCtx, cancel, err := getFullGrpcCtx()
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Purchase(grpcCtx, &papi.ShopPurchaseRequest{ShopItemId: tc.item})
			cancel()
			if tc.failure {
				if err == nil || tc.item != "test-diamond-rollback" && status.Code(err) != codes.ResourceExhausted {
					t.Fatalf("Purchase error = %v", err)
				}
				purchaseAfter, afterErr := opuser.FindUserShopPurchase(ctx, testConfig.TestUserSuid)
				if !reflect.DeepEqual(purchaseBefore, purchaseAfter) || (beforeErr == nil) != (afterErr == nil) {
					t.Fatalf("failed purchase changed purchase state: before=%v after=%v", purchaseBefore, purchaseAfter)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				assertDiamondResponseBalance(t, response.GetCommonResponse().GetUpdatedData().GetBalance(), tc.wantFree, tc.wantPaid)
			}
			goldAfter, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, penum.PointType_PointType_Gold)
			if err != nil {
				t.Fatal(err)
			}
			wantGold := goldBefore.GetAmount()
			if !tc.failure {
				wantGold++
			}
			if goldAfter.GetAmount() != wantGold {
				t.Fatalf("gold after purchase = %d, want %d", goldAfter.GetAmount(), wantGold)
			}
			assertDiamondStoredBalance(t, tc.wantFree, tc.wantPaid)
		})
	}
}

func setDiamondTestBalance(t *testing.T, free, paid int32) {
	t.Helper()
	if err := opuser.UpsertUserBalance(context.Background(), testConfig.TestUserSuid, &ptransaction.UserBalance{FreeBalance: free, PaidBalance: paid}); err != nil {
		t.Fatal(err)
	}
}

func assertDiamondStoredBalance(t *testing.T, free, paid int32) {
	t.Helper()
	balance, err := opuser.FindUserBalance(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	assertDiamondResponseBalance(t, balance, free, paid)
}

func assertDiamondResponseBalance(t *testing.T, balance *ptransaction.UserBalance, free, paid int32) {
	t.Helper()
	if balance == nil || balance.GetFreeBalance() != free || balance.GetPaidBalance() != paid {
		t.Fatalf("balance = %v, want free=%d paid=%d", balance, free, paid)
	}
}
