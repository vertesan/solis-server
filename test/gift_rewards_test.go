package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestGiftReceiveAggregatesRewards(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	assertGiftNotification(t, false)

	items, err := opmaster.FindAllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Skip("master database has no item suitable for a gift")
	}
	itemID := items[0].GetId()
	before := userItemAmount(t, itemID)
	gifts := []*ptransaction.UserGift{
		newTestGift(penum.ResourceType_ResourceType_Item, itemID, 2),
		newTestGift(penum.ResourceType_ResourceType_Item, itemID, 5),
	}
	for _, gift := range gifts {
		if err := opuser.UpsertUserGift(ctx, testConfig.TestUserSuid, gift); err != nil {
			t.Fatal(err)
		}
	}

	response := receiveTestGifts(t, gifts[0].GetGiftId(), gifts[1].GetGiftId())
	if len(response.GetResults()) != 1 {
		t.Fatalf("Gift.Receive returned %d results for one repeated resource, want 1: %v", len(response.GetResults()), response.GetResults())
	}
	result := response.GetResults()[0]
	if result.GetResourceType() != penum.ResourceType_ResourceType_Item || result.GetResourceId() != itemID ||
		result.GetBeforeAmount() != before || result.GetAfterAmount() != before+7 || result.GetAmountI64() != 7 {
		t.Fatalf("Gift.Receive returned incorrect aggregated result: %v", result)
	}
	if got := userItemAmount(t, itemID); got != before+7 {
		t.Fatalf("Gift.Receive item amount = %d, want %d", got, before+7)
	}
	if len(response.GetReceivedGiftIds()) != len(gifts) || response.GetHasUnreceivedGift() {
		t.Fatalf("Gift.Receive returned incorrect mailbox state: %v", response)
	}
	assertGiftNotification(t, false)
}

func TestGiftExpiringItem(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	items, err := opmaster.FindAllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Skip("master database has no item suitable for a gift")
	}
	itemID := items[0].GetId()
	before := activeUserItemAmount(t, itemID)
	expiredTime := time.Now().Add(24 * time.Hour).UnixMilli()
	gift := newTestGift(penum.ResourceType_ResourceType_Item, itemID, 3)
	gift.LimitTime = expiredTime
	if err := opuser.UpsertUserGift(ctx, testConfig.TestUserSuid, gift); err != nil {
		t.Fatal(err)
	}

	response := receiveTestGifts(t, gift.GetGiftId())
	userItems, err := opuser.FindUserItems(ctx, testConfig.TestUserSuid, itemID)
	if err != nil {
		t.Fatal(err)
	}
	stored := findUserItem(userItems, itemID, expiredTime)
	if stored == nil || stored.GetAmount() != gift.GetAmount() {
		t.Fatalf("Gift.Receive expiring item = %v, want amount %d at expiration %d", stored, gift.GetAmount(), expiredTime)
	}
	result := findRewardResult(response.GetResults(), penum.ResourceType_ResourceType_Item, itemID)
	if result == nil || result.GetBeforeAmount() != before || result.GetAfterAmount() != before+gift.GetAmount() {
		t.Fatalf("Gift.Receive expiring item result = %v, want balance %d -> %d", result, before, before+gift.GetAmount())
	}
	updated := response.GetCommonResponse().GetUpdatedData()
	if findUserItem(updated.GetItems(), itemID, expiredTime) == nil {
		t.Fatalf("Gift.Receive omitted expiring item from updated data: %v", updated.GetItems())
	}
}

func TestGiftCostumeGrantsDefaultHair(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	costumes, err := opmaster.FindAllCostumes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var costumeID, hairID string
	for _, costume := range costumes {
		if costume.GetDefaultHairId() == "" {
			continue
		}
		if _, err := opmaster.FindHair(ctx, costume.GetDefaultHairId()); err != nil {
			continue
		}
		_, costumeErr := opuser.FindUserCostume(ctx, testConfig.TestUserSuid, costume.GetId())
		_, hairErr := opuser.FindUserHair(ctx, testConfig.TestUserSuid, costume.GetDefaultHairId())
		if errors.Is(costumeErr, mongo.ErrNoDocuments) && errors.Is(hairErr, mongo.ErrNoDocuments) {
			costumeID, hairID = costume.GetId(), costume.GetDefaultHairId()
			break
		}
	}
	if costumeID == "" {
		t.Skip("master database has no unowned costume with an unowned default hair")
	}

	gift := newTestGift(penum.ResourceType_ResourceType_Costume, costumeID, 1)
	if err := opuser.UpsertUserGift(ctx, testConfig.TestUserSuid, gift); err != nil {
		t.Fatal(err)
	}
	response := receiveTestGifts(t, gift.GetGiftId())
	if findRewardResult(response.GetResults(), penum.ResourceType_ResourceType_Costume, costumeID) == nil ||
		findRewardResult(response.GetResults(), penum.ResourceType_ResourceType_Hair, hairID) == nil {
		t.Fatalf("Gift.Receive omitted costume or default-hair result: %v", response.GetResults())
	}
	if _, err := opuser.FindUserCostume(ctx, testConfig.TestUserSuid, costumeID); err != nil {
		t.Fatalf("Gift.Receive did not persist costume %q: %v", costumeID, err)
	}
	hair, err := opuser.FindUserHair(ctx, testConfig.TestUserSuid, hairID)
	if err != nil || !hair.GetChecked() {
		t.Fatalf("Gift.Receive default hair = %v, err = %v; want persisted and checked", hair, err)
	}
	updated := response.GetCommonResponse().GetUpdatedData()
	if findUserHair(updated.GetHairs(), hairID) == nil || !findUserHair(updated.GetHairs(), hairID).GetChecked() {
		t.Fatalf("Gift.Receive omitted checked default hair from updated data: %v", updated.GetHairs())
	}
}

func TestGiftGoldUpdatesAccountingAndMissions(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	pointBefore, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil {
		t.Fatal(err)
	}
	totalBefore, err := opuser.FindUserTotalCount(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}

	missions, err := opmaster.FindAllMissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var masterMissionID string
	var threshold, maxThreshold int64
	for _, mission := range missions {
		if mission.GetMissionType() != penum.MissionType_MissionType_PointGoldAmount || len(mission.GetTargetIds()) != 0 {
			continue
		}
		for _, progress := range mission.GetProgresses() {
			if progress.GetThreshold() <= 0 {
				continue
			}
			if threshold == 0 || progress.GetThreshold() < threshold {
				threshold = progress.GetThreshold()
			}
			if progress.GetThreshold() > maxThreshold {
				maxThreshold = progress.GetThreshold()
			}
		}
		if threshold > 0 {
			masterMissionID = mission.GetId()
			break
		}
	}
	if masterMissionID == "" {
		t.Skip("master database has no untargeted PointGoldAmount mission")
	}
	if err := opuser.UpsertUserMission(ctx, testConfig.TestUserSuid, &ptransaction.UserMission{
		MissionId:        masterMissionID,
		CurrentThreshold: threshold,
		StatusType:       penum.MissionStatusType_MissionStatusType_Progress,
	}); err != nil {
		t.Fatal(err)
	}

	const amount int64 = 7
	gift := newTestGift(penum.ResourceType_ResourceType_Gold, "", amount)
	if err := opuser.UpsertUserGift(ctx, testConfig.TestUserSuid, gift); err != nil {
		t.Fatal(err)
	}
	response := receiveTestGifts(t, gift.GetGiftId())

	pointAfter, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil || pointAfter.GetAmount() != pointBefore.GetAmount()+amount {
		t.Fatalf("Gift.Receive gold point = %v, err = %v; want amount %d", pointAfter, err, pointBefore.GetAmount()+amount)
	}
	totalAfter, err := opuser.FindUserTotalCount(ctx, testConfig.TestUserSuid)
	if err != nil || totalAfter.GetPointGoldAmount() != totalBefore.GetPointGoldAmount()+amount {
		t.Fatalf("Gift.Receive total earned gold = %v, err = %v; want %d", totalAfter, err, totalBefore.GetPointGoldAmount()+amount)
	}
	storedMission, err := findUserMission(ctx, masterMissionID)
	wantProgress := min(amount, maxThreshold)
	if err != nil || storedMission.GetProgress() != wantProgress {
		t.Fatalf("Gift.Receive PointGoldAmount mission = %v, err = %v; want progress %d", storedMission, err, wantProgress)
	}
	updated := response.GetCommonResponse().GetUpdatedData()
	if updated.GetTotalCount().GetPointGoldAmount() != totalAfter.GetPointGoldAmount() || findUserMissionByID(updated.GetMissions(), masterMissionID) == nil {
		t.Fatalf("Gift.Receive omitted total count or mission from updated data: %v", updated)
	}
}

func TestGiftCardRewards(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	cards, err := opmaster.FindAllCards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var cardID string
	now := time.Now().UnixMilli()
	for _, card := range cards {
		if card.GetInitialRarity() <= 0 || card.GetReleaseDate() > now {
			continue
		}
		if _, err := opuser.FindUserCard(ctx, testConfig.TestUserSuid, card.GetId()); errors.Is(err, mongo.ErrNoDocuments) {
			cardID = card.GetId()
			break
		}
	}
	if cardID == "" {
		t.Skip("master database has no released unowned card")
	}

	cardGift := newTestGift(penum.ResourceType_ResourceType_Card, cardID, 1)
	if err := opuser.UpsertUserGift(ctx, testConfig.TestUserSuid, cardGift); err != nil {
		t.Fatal(err)
	}
	cardResponse := receiveTestGifts(t, cardGift.GetGiftId())
	if findRewardResult(cardResponse.GetResults(), penum.ResourceType_ResourceType_Card, cardID) == nil {
		t.Fatalf("Gift.Receive omitted card result: %v", cardResponse.GetResults())
	}
	card, err := opuser.FindUserCard(ctx, testConfig.TestUserSuid, cardID)
	if err != nil {
		t.Fatalf("Gift.Receive did not persist card %q: %v", cardID, err)
	}

	const rarityExpAmount int64 = 1
	before := card.GetRarityTotalExp()
	expGift := newTestGift(penum.ResourceType_ResourceType_CardRarityExpLimitChange, cardID, rarityExpAmount)
	if err := opuser.UpsertUserGift(ctx, testConfig.TestUserSuid, expGift); err != nil {
		t.Fatal(err)
	}
	expResponse := receiveTestGifts(t, expGift.GetGiftId())
	result := findRewardResult(expResponse.GetResults(), penum.ResourceType_ResourceType_CardRarityExpLimitChange, cardID)
	if result == nil || result.GetBeforeAmount() != int64(before) || result.GetAfterAmount() != int64(before)+rarityExpAmount {
		t.Fatalf("Gift.Receive returned incorrect rarity-limit result: %v", expResponse.GetResults())
	}
	card, err = opuser.FindUserCard(ctx, testConfig.TestUserSuid, cardID)
	if err != nil || int64(card.GetRarityTotalExp()) != int64(before)+rarityExpAmount {
		t.Fatalf("Gift.Receive card rarity total exp = %v, err = %v; want %d", card, err, int64(before)+rarityExpAmount)
	}
}

func newTestGift(resourceType penum.ResourceType, resourceID string, amount int64) *ptransaction.UserGift {
	return &ptransaction.UserGift{
		GiftId:       uuid.NewString(),
		ResourceType: resourceType,
		ResourceId:   resourceID,
		Amount:       amount,
		Message:      "integration test gift",
		PostedTime:   time.Now().UnixMilli(),
	}
}

func receiveTestGifts(t *testing.T, giftIDs ...string) *papi.GiftReceiveResponse {
	t.Helper()
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	response, err := papi.NewGiftClient(testConfig.GrpcConn).Receive(grpcCtx, &papi.GiftReceiveRequest{GiftIds: giftIDs})
	if err != nil {
		t.Fatalf("Gift.Receive failed: %v", err)
	}
	if response.GetCommonResponse() == nil || response.GetCommonResponse().GetUpdatedData() == nil {
		t.Fatalf("Gift.Receive returned incomplete response: %v", response)
	}
	return response
}

func assertGiftNotification(t *testing.T, want bool) {
	t.Helper()
	notifications, err := opuser.FindAllUserNotifications(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, notification := range notifications {
		if notification.GetNotificationType() == penum.NotificationType_NotificationType_Gift {
			if notification.GetValid() != want {
				t.Fatalf("gift notification valid = %t, want %t", notification.GetValid(), want)
			}
			return
		}
	}
	t.Fatal("gift notification is missing")
}

func findRewardResult(results []*ptransaction.RewardResult, resourceType penum.ResourceType, resourceID string) *ptransaction.RewardResult {
	for _, result := range results {
		if result.GetResourceType() == resourceType && result.GetResourceId() == resourceID {
			return result
		}
	}
	return nil
}

func findUserHair(hairs []*ptransaction.UserHair, hairID string) *ptransaction.UserHair {
	for _, hair := range hairs {
		if hair.GetHairId() == hairID {
			return hair
		}
	}
	return nil
}

func findUserItem(items []*ptransaction.UserItem, itemID string, expiredTime int64) *ptransaction.UserItem {
	for _, item := range items {
		if item.GetItemId() == itemID && item.GetExpiredTime() == expiredTime {
			return item
		}
	}
	return nil
}

func activeUserItemAmount(t *testing.T, itemID string) int64 {
	t.Helper()
	items, err := opuser.FindUserItems(context.Background(), testConfig.TestUserSuid, itemID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	var amount int64
	for _, item := range items {
		if item.GetExpiredTime() == 0 || item.GetExpiredTime() > now {
			amount += item.GetAmount()
		}
	}
	return amount
}

func findUserMission(ctx context.Context, missionID string) (*ptransaction.UserMission, error) {
	missions, err := opuser.FindAllUserMissions(ctx, testConfig.TestUserSuid)
	if err != nil {
		return nil, err
	}
	mission := findUserMissionByID(missions, missionID)
	if mission == nil {
		return nil, mongo.ErrNoDocuments
	}
	return mission, nil
}

func findUserMissionByID(missions []*ptransaction.UserMission, missionID string) *ptransaction.UserMission {
	for _, mission := range missions {
		if mission.GetMissionId() == missionID {
			return mission
		}
	}
	return nil
}
