package test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGiftAPIs(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	masterItems, err := opmaster.FindAllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(masterItems) == 0 {
		t.Skip("master database has no item suitable for a gift")
	}
	itemID := masterItems[0].GetId()
	gift := &ptransaction.UserGift{
		GiftId:       uuid.NewString(),
		ResourceType: 1,
		ResourceId:   itemID,
		Amount:       3,
		Message:      "integration test gift",
		PostedTime:   time.Now().UnixMilli(),
	}
	if err := opuser.UpsertUserGift(ctx, testConfig.TestUserSuid, gift); err != nil {
		t.Fatal(err)
	}
	before := userItemAmount(t, itemID)
	client := papi.NewGiftClient(testConfig.GrpcConn)

	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	list, err := client.List(grpcCtx, &papi.GiftListRequest{})
	cancel()
	if err != nil {
		t.Fatalf("Gift.List failed: %v", err)
	}
	if list.GetCommonResponse() == nil || !hasGift(list.GetGifts(), gift.GetGiftId()) {
		t.Fatalf("Gift.List omitted the current gift: %v", list)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	received, err := client.Receive(grpcCtx, &papi.GiftReceiveRequest{GiftIds: []string{gift.GetGiftId()}})
	cancel()
	if err != nil {
		t.Fatalf("Gift.Receive failed: %v", err)
	}
	if received.GetCommonResponse() == nil || len(received.GetResults()) != 1 ||
		len(received.GetReceivedGiftIds()) != 1 || received.GetReceivedGiftIds()[0] != gift.GetGiftId() {
		t.Fatalf("Gift.Receive returned incomplete result: %v", received)
	}
	if got := userItemAmount(t, itemID); got != before+gift.GetAmount() {
		t.Fatalf("Gift.Receive item amount = %d, want %d", got, before+gift.GetAmount())
	}
	if _, err := opuser.FindUserGift(ctx, testConfig.TestUserSuid, gift.GetGiftId()); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("received gift still exists: %v", err)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	history, err := client.ListHistory(grpcCtx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("Gift.ListHistory failed: %v", err)
	}
	if history.GetCommonResponse() == nil || !hasGiftHistory(history.GetGifts(), gift.GetGiftId()) {
		t.Fatalf("Gift.ListHistory omitted the received gift: %v", history)
	}
}

func TestLoginBonusAPIs(t *testing.T) {
	setupTestUser(t)
	client := papi.NewLoginBonusClient(testConfig.GrpcConn)
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	list, err := client.List(grpcCtx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("LoginBonus.List failed: %v", err)
	}
	if list.GetCommonResponse() == nil {
		t.Fatalf("LoginBonus.List returned incomplete response: %v", list)
	}
	for _, info := range list.GetInfos() {
		assertDummyNoticeInfo(t, "LoginBonus.List "+info.GetUserLoginBonus().GetLoginBonusId(), info.GetNoticeInfo())
	}
	var candidate *ptransaction.UserLoginBonus
	for _, info := range list.GetInfos() {
		state := info.GetUserLoginBonus()
		if state != nil && state.GetReceiveCount() < state.GetLoginCount() {
			candidate = state
			break
		}
	}
	if candidate == nil {
		t.Skip("current master rotation has no receivable login bonus")
	}
	masterBonus, err := opmaster.FindLoginBonus(context.Background(), candidate.GetLoginBonusId())
	if err != nil {
		t.Fatalf("failed to reload listed login bonus master: %v", err)
	}
	rewardIndex := int(candidate.GetReceiveCount())
	if masterBonus.GetType() == penum.LoginBonusType_LoginBonusType_Normal {
		rewardIndex %= len(masterBonus.GetRewardIds())
	}
	if rewardIndex >= len(masterBonus.GetRewardIds()) {
		t.Fatalf("listed login bonus has no reward at receive count %d: %v", rewardIndex, masterBonus)
	}
	if !strings.HasPrefix(masterBonus.GetRewardIds()[rewardIndex], "rd_auto;") {
		if _, err := opmaster.FindReward(context.Background(), masterBonus.GetRewardIds()[rewardIndex]); err != nil {
			t.Fatalf("listed login bonus references missing reward %q: %v", masterBonus.GetRewardIds()[rewardIndex], err)
		}
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	repeatedList, err := client.List(grpcCtx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("second LoginBonus.List failed: %v", err)
	}
	repeated := findLoginBonusState(repeatedList.GetInfos(), candidate.GetLoginBonusId())
	if repeated == nil || repeated.GetLoginCount() != candidate.GetLoginCount() {
		t.Fatalf("LoginBonus.List advanced twice in one JST day: first=%v second=%v", candidate, repeated)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	received, err := client.Receive(grpcCtx, &papi.LoginBonusReceiveRequest{LoginBonusId: candidate.GetLoginBonusId()})
	cancel()
	if err != nil {
		t.Fatalf("LoginBonus.Receive failed: %v", err)
	}
	if received.GetCommonResponse() == nil || received.GetUserLoginBonus() == nil || len(received.GetRewards()) == 0 ||
		received.GetUserLoginBonus().GetReceiveCount() != candidate.GetReceiveCount()+1 {
		t.Fatalf("LoginBonus.Receive returned incomplete state: %v", received)
	}
	stored, err := opuser.FindUserLoginBonus(context.Background(), testConfig.TestUserSuid, candidate.GetLoginBonusId())
	if err != nil || stored.GetReceiveCount() != received.GetUserLoginBonus().GetReceiveCount() {
		t.Fatalf("LoginBonus.Receive did not persist state: stored=%v response=%v err=%v", stored, received.GetUserLoginBonus(), err)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Receive(grpcCtx, &papi.LoginBonusReceiveRequest{LoginBonusId: candidate.GetLoginBonusId()})
	cancel()
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("duplicate LoginBonus.Receive returned %v, want FailedPrecondition", err)
	}
}

func hasGift(gifts []*ptransaction.UserGift, giftID string) bool {
	for _, gift := range gifts {
		if gift.GetGiftId() == giftID {
			return true
		}
	}
	return false
}

func hasGiftHistory(gifts []*ptransaction.UserGiftHistory, giftID string) bool {
	for _, gift := range gifts {
		if gift.GetGiftId() == giftID {
			return true
		}
	}
	return false
}

func findLoginBonusState(infos []*papi.LoginBonusInfo, loginBonusID string) *ptransaction.UserLoginBonus {
	for _, info := range infos {
		if info.GetUserLoginBonus().GetLoginBonusId() == loginBonusID {
			return info.GetUserLoginBonus()
		}
	}
	return nil
}
