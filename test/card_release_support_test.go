package test

import (
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func TestCardReleaseSupportScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewCardClient(testConfig.GrpcConn)
	before, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	firstCost, err := opmaster.FindCardSupportRelease(ctx, before.GetCardSupportReleaseCount()+1)
	if err != nil {
		t.Fatal(err)
	}
	setCardTrainingItem(t, firstCost.GetItemId(), 0)
	profileReject(t, codes.ResourceExhausted, client.ReleaseSupport, &papi.CardReleaseSupportRequest{})
	unchanged, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil || !proto.Equal(before, unchanged) {
		t.Fatalf("failed release changed slot state: %v", err)
	}
	setCardTrainingItem(t, firstCost.GetItemId(), int64(firstCost.GetItemAmount())+9)
	first := profileCall(t, client.ReleaseSupport, &papi.CardReleaseSupportRequest{})
	if first.GetCommonResponse().GetUpdatedData().GetUser().GetCardSupportMaxNumber() != before.GetCardSupportMaxNumber()+1 ||
		first.GetCommonResponse().GetUpdatedData().GetUser().GetCardSupportReleaseCount() != before.GetCardSupportReleaseCount()+1 ||
		userItemAmount(t, firstCost.GetItemId()) != 9 {
		t.Fatalf("point release did not debit master cost and unlock one slot: %v", first)
	}
	secondCost, err := opmaster.FindCardSupportRelease(ctx, before.GetCardSupportReleaseCount()+2)
	if err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserBalance(ctx, testConfig.TestUserSuid, &ptransaction.UserBalance{FreeBalance: secondCost.GetStoneAmount() - 1}); err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.ResourceExhausted, client.ReleaseSupport, &papi.CardReleaseSupportRequest{UseStone: true})
	if err := opuser.UpsertUserBalance(ctx, testConfig.TestUserSuid, &ptransaction.UserBalance{FreeBalance: secondCost.GetStoneAmount() - 1, PaidBalance: 2}); err != nil {
		t.Fatal(err)
	}
	second := profileCall(t, client.ReleaseSupport, &papi.CardReleaseSupportRequest{UseStone: true})
	if second.GetCommonResponse().GetUpdatedData().GetBalance().GetFreeBalance() != 0 || second.GetCommonResponse().GetUpdatedData().GetBalance().GetPaidBalance() != 1 ||
		second.GetCommonResponse().GetUpdatedData().GetUser().GetCardSupportMaxNumber() != before.GetCardSupportMaxNumber()+2 || userItemAmount(t, firstCost.GetItemId()) != 9 {
		t.Fatalf("diamond release did not use free then paid diamonds: %v", second)
	}
	userGet := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	if userGet.GetUser().GetCardSupportReleaseCount() != before.GetCardSupportReleaseCount()+2 || userGet.GetUser().GetCardSupportMaxNumber() != before.GetCardSupportMaxNumber()+2 {
		t.Fatal("User.Get lost released support slots")
	}
	// Cross the first price boundary with concurrent purchases. Each request
	// must price the next slot after its transaction commits/retries.
	pointCosts := make(map[string]int64)
	for _, next := range []int32{before.GetCardSupportReleaseCount() + 3, before.GetCardSupportReleaseCount() + 4} {
		cost, err := opmaster.FindCardSupportRelease(ctx, next)
		if err != nil {
			t.Fatal(err)
		}
		pointCosts[cost.GetItemId()] += int64(cost.GetItemAmount())
	}
	for id, amount := range pointCosts {
		setCardTrainingItem(t, id, amount)
	}
	results := make(chan error, 2)
	for range 2 {
		callCtx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			defer cancel()
			_, err := client.ReleaseSupport(callCtx, &papi.CardReleaseSupportRequest{})
			results <- err
		}()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent support release failed: %v", err)
		}
	}
	for id := range pointCosts {
		if userItemAmount(t, id) != 0 {
			t.Fatal("concurrent support releases did not charge both successive master prices")
		}
	}
	userGet = profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	if userGet.GetUser().GetCardSupportReleaseCount() != before.GetCardSupportReleaseCount()+4 || userGet.GetUser().GetCardSupportMaxNumber() != before.GetCardSupportMaxNumber()+4 {
		t.Fatal("concurrent support release lost or duplicated a slot")
	}
	maximum, err := opmaster.FindMaxCardSupportRelease(ctx)
	if err != nil {
		t.Fatal(err)
	}
	user := userGet.GetUser()
	user.CardSupportReleaseCount, user.CardSupportMaxNumber = maximum.GetCount(), maximum.GetCount()+3
	if err := opuser.UpsertUser(ctx, testConfig.TestUserSuid, user); err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.FailedPrecondition, client.ReleaseSupport, &papi.CardReleaseSupportRequest{UseStone: true})
	if userItemAmount(t, firstCost.GetItemId()) != 0 {
		t.Fatal("release above the master cap consumed resources")
	}
}
