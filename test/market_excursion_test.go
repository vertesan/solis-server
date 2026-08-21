package test

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type missionProgressExpectation struct {
	missionID     string
	before        int64
	maximum       int64
	currentTarget int64
}

func TestMarketScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)

	client := papi.NewMarketClient(testConfig.GrpcConn)
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	listed, err := client.ListItem(ctx, &papi.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if listed.GetCommonResponse() == nil || len(listed.GetMarketItems()) == 0 {
		t.Fatalf("market list is incomplete: %v", listed)
	}
	now := time.Now().UnixMilli()
	if listed.GetNextResetTime() <= now || listed.GetNextResetTime() > now+12*int64(time.Hour/time.Millisecond) {
		t.Fatalf("market next reset is not the next half-day boundary: %d", listed.GetNextResetTime())
	}

	purchaseItem, rewardID, rewardAmount := marketItemReward(t, listed.GetMarketItems())
	missionExpectation := activateMatchingMission(t, penum.MissionType_MissionType_MarketExchangeCount, "")
	goldBefore, err := opuser.FindUserPoint(context.Background(), testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil {
		t.Fatal(err)
	}
	totalBefore, err := opuser.FindUserTotalCount(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	itemBefore := userItemAmount(t, rewardID)

	purchased, err := client.Purchase(ctx, &papi.MarketPurchaseRequest{Number: purchaseItem.GetNumber()})
	if err != nil {
		t.Fatal(err)
	}
	if purchased.GetCommonResponse() == nil || !marketResponseItemSoldOut(purchased.GetMarketItems(), purchaseItem.GetNumber()) {
		t.Fatalf("purchased item was not returned as sold out: %v", purchased)
	}
	goldAfter, err := opuser.FindUserPoint(context.Background(), testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil {
		t.Fatal(err)
	}
	if goldAfter.GetAmount() != goldBefore.GetAmount()-int64(purchaseItem.GetDiscountAmount()) {
		t.Fatalf("market gold cost mismatch: before=%d after=%d cost=%d", goldBefore.GetAmount(), goldAfter.GetAmount(), purchaseItem.GetDiscountAmount())
	}
	if itemAfter := userItemAmount(t, rewardID); itemAfter != itemBefore+rewardAmount {
		t.Fatalf("market reward mismatch for %s: before=%d after=%d reward=%d", rewardID, itemBefore, itemAfter, rewardAmount)
	}
	totalAfter, err := opuser.FindUserTotalCount(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if totalAfter.GetPointGoldAmount() != totalBefore.GetPointGoldAmount() ||
		totalAfter.GetMarketExchangeCount() != totalBefore.GetMarketExchangeCount()+1 {
		t.Fatalf("market total-count mismatch: before=%v after=%v", totalBefore, totalAfter)
	}
	updated := purchased.GetCommonResponse().GetUpdatedData()
	if updated.GetTotalCount() == nil || !proto.Equal(updated.GetTotalCount(), totalAfter) {
		t.Fatalf("market purchase returned an incorrect total-count post-image: response=%v stored=%v", updated.GetTotalCount(), totalAfter)
	}
	assertMissionAdvanced(t, missionExpectation, 1, updated.GetMissions())
	state, err := opuser.FindUserMarketState(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !marketResponseItemSoldOut(state.MarketItems, purchaseItem.GetNumber()) {
		t.Fatal("market purchase was not persisted")
	}
	if _, err := client.Purchase(ctx, &papi.MarketPurchaseRequest{Number: purchaseItem.GetNumber()}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("duplicate market purchase returned %v, want %v", err, codes.FailedPrecondition)
	}

	reset, err := client.Reset(ctx, &papi.MarketResetRequest{IsFree: true})
	if err != nil {
		t.Fatal(err)
	}
	if reset.GetCommonResponse() == nil || reset.GetFreeResetRemainCount() != 0 || len(reset.GetMarketItems()) == 0 {
		t.Fatalf("unexpected free market reset response: %v", reset)
	}
	for _, item := range reset.GetMarketItems() {
		if item.GetSoldOut() {
			t.Fatalf("fresh market inventory contains a sold-out item: %v", item)
		}
	}
	if _, err := client.Reset(ctx, &papi.MarketResetRequest{IsFree: true}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("second free market reset returned %v, want %v", err, codes.FailedPrecondition)
	}
	balanceBefore, err := opuser.FindUserBalance(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	paidReset, err := client.Reset(ctx, &papi.MarketResetRequest{IsFree: false})
	if err != nil {
		t.Fatal(err)
	}
	balanceAfter, err := opuser.FindUserBalance(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if balanceAfter.GetFreeBalance() != balanceBefore.GetFreeBalance()-paidReset.GetRequiredResetStoneAmount() {
		t.Fatalf("paid market reset stone cost mismatch: before=%d after=%d cost=%d", balanceBefore.GetFreeBalance(), balanceAfter.GetFreeBalance(), paidReset.GetRequiredResetStoneAmount())
	}
	if paidReset.GetResetRemainCount() != reset.GetResetRemainCount()-1 {
		t.Fatalf("paid market reset count mismatch: before=%d after=%d", reset.GetResetRemainCount(), paidReset.GetResetRemainCount())
	}
}

func TestExcursionScenario(t *testing.T) {
	setupTestUser(t)
	client := papi.NewExcursionClient(testConfig.GrpcConn)
	lockedCharacterID := findExcursionCharacter(t)
	lockedCtx, lockedCancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.List(lockedCtx, &papi.ExcursionListRequest{CharacterId: lockedCharacterID})
	lockedCancel()
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("locked Excursion.List returned %v, want %v", err, codes.PermissionDenied)
	}

	promoteTestUser(t)
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	characterID, placeID := listExcursionOffer(t, ctx, client)
	setting, err := opmaster.FindExcursionPlaceSetting(context.Background(), characterID, placeID)
	if err != nil {
		t.Fatal(err)
	}
	costume, hairID := excursionWardrobe(t, characterID, setting)
	reactionNumbers := compatibleExcursionReactionNumbers(t, characterID, placeID, costume.GetId())
	missionExpectation := activateMatchingMission(t, penum.MissionType_MissionType_ExcursionClearCount, characterID)
	giftItem, err := opmaster.FindExcursionGiftItem(context.Background(), characterID)
	if err != nil {
		t.Fatal(err)
	}

	started, err := client.Start(ctx, &papi.ExcursionStartRequest{
		CharacterId:      characterID,
		ExcursionPlaceId: placeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.GetCommonResponse() == nil || len(started.GetRewardResults()) != 0 {
		t.Fatalf("unexpected excursion start response: %v", started)
	}
	active, err := opuser.FindUserExcursionState(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if active.ActiveCharacter != characterID || active.ActivePlaceId != placeID || active.StartedTime <= 0 ||
		active.OfferedCharacter != "" || len(active.OfferedPlaceIds) != 0 {
		t.Fatalf("excursion start was not persisted: %+v", active)
	}

	giftsBefore, err := opuser.FindAllUserGifts(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	finished, err := client.Finish(ctx, &papi.ExcursionFinishRequest{
		GazeReactionNumbers: reactionNumbers,
		CostumeId:           costume.GetId(),
		HairId:              hairID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if finished.GetCommonResponse() == nil || len(finished.GetRewardResults()) != 1 {
		t.Fatalf("unexpected excursion finish response: %v", finished)
	}
	reward := finished.GetRewardResults()[0]
	if !reward.GetIsGift() || reward.GetResourceType() != penum.ResourceType_ResourceType_Item || reward.GetResourceId() != giftItem.GetId() || reward.GetAmountI64() != 1 {
		t.Fatalf("unexpected excursion reward: %v", reward)
	}
	assertMissionAdvanced(t, missionExpectation, 1, finished.GetCommonResponse().GetUpdatedData().GetMissions())
	giftsAfter, err := opuser.FindAllUserGifts(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(giftsAfter) != len(giftsBefore)+1 || !containsExcursionGift(giftsAfter, reward.GetResourceId()) {
		t.Fatalf("excursion gift was not persisted: before=%d after=%v", len(giftsBefore), giftsAfter)
	}
	completed, err := opuser.FindUserExcursionState(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if completed.ActiveCharacter != "" || completed.ActivePlaceId != "" || completed.StartedTime != 0 {
		t.Fatalf("finished excursion remained active: %+v", completed)
	}
	for _, number := range reactionNumbers {
		if _, err := opuser.FindUserExcursionGazeReaction(context.Background(), testConfig.TestUserSuid, characterID, placeID, int64(number)); err != nil {
			t.Fatalf("excursion gaze reaction %d was not persisted: %v", number, err)
		}
	}
	if _, err := client.Finish(ctx, &papi.ExcursionFinishRequest{CostumeId: costume.GetId(), HairId: hairID}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("second excursion finish returned %v, want %v", err, codes.FailedPrecondition)
	}
}

func findExcursionCharacter(t *testing.T) string {
	t.Helper()
	characters, err := opuser.FindAllUserCharacters(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, character := range characters {
		settings, err := opmaster.FindExcursionPlaceSettingsByCharacter(context.Background(), character.GetCharacterId())
		if err == nil && len(settings) > 0 {
			return character.GetCharacterId()
		}
	}
	t.Fatal("current master/user data has no excursion character")
	return ""
}

func marketItemReward(t *testing.T, items []*papi.MarketItem) (*papi.MarketItem, string, int64) {
	t.Helper()
	for _, item := range items {
		parts := strings.Split(item.GetRewardId(), ";")
		if item.GetConsumptionResourceType() != penum.ResourceType_ResourceType_Gold || len(parts) != 4 || parts[0] != "rd_auto" || parts[1] != "item" {
			continue
		}
		amount, err := strconv.ParseInt(parts[3], 10, 64)
		if err == nil && amount > 0 {
			return item, parts[2], amount
		}
	}
	t.Fatal("market did not return a gold-priced item reward")
	return nil, "", 0
}

func marketResponseItemSoldOut(items []*papi.MarketItem, number int32) bool {
	for _, item := range items {
		if item.GetNumber() == number {
			return item.GetSoldOut()
		}
	}
	return false
}

func listExcursionOffer(t *testing.T, ctx context.Context, client papi.ExcursionClient) (string, string) {
	t.Helper()
	characters, err := opuser.FindAllUserCharacters(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, character := range characters {
		settings, err := opmaster.FindExcursionPlaceSettingsByCharacter(context.Background(), character.GetCharacterId())
		if err != nil || len(settings) == 0 {
			continue
		}
		response, err := client.List(ctx, &papi.ExcursionListRequest{CharacterId: character.GetCharacterId()})
		if err != nil || len(response.GetExcursionPlaceIds()) == 0 {
			continue
		}
		if response.GetCommonResponse() == nil {
			t.Fatalf("excursion list omitted common response: %v", response)
		}
		allowed := make(map[string]struct{}, len(settings))
		for _, setting := range settings {
			allowed[setting.GetExcursionPlaceId()] = struct{}{}
		}
		seen := make(map[string]struct{}, len(response.GetExcursionPlaceIds()))
		for _, placeID := range response.GetExcursionPlaceIds() {
			if _, ok := allowed[placeID]; !ok {
				t.Fatalf("excursion list returned %q outside current master settings", placeID)
			}
			if _, duplicate := seen[placeID]; duplicate {
				t.Fatalf("excursion list returned duplicate place %q", placeID)
			}
			seen[placeID] = struct{}{}
		}
		if len(response.GetExcursionPlaceIds()) != min(2, len(allowed)) {
			t.Fatalf("excursion list returned %d places, want %d", len(response.GetExcursionPlaceIds()), min(2, len(allowed)))
		}
		return character.GetCharacterId(), response.GetExcursionPlaceIds()[0]
	}
	t.Fatal("current master/user data has no available excursion")
	return "", ""
}

func excursionWardrobe(t *testing.T, characterID string, setting *pmaster.ExcursionPlaceSetting) (*pmaster.Costume, string) {
	t.Helper()
	costumeIDs := []string{setting.GetForceCostumeId(), setting.GetAdvForceCostumeId(), setting.GetFinishAdvForceCostumeId()}
	for _, costumeID := range costumeIDs {
		if costumeID == "" {
			continue
		}
		costume, err := opmaster.FindCostume(context.Background(), costumeID)
		if err != nil || costume.GetCharacterId() != characterID || costume.GetDefaultHairId() == "" {
			continue
		}
		if _, err := opuser.FindUserCostume(context.Background(), testConfig.TestUserSuid, costumeID); err != nil {
			continue
		}
		if _, err := opuser.FindUserHair(context.Background(), testConfig.TestUserSuid, costume.GetDefaultHairId()); err != nil {
			continue
		}
		return costume, costume.GetDefaultHairId()
	}
	t.Fatal("current master/user data has no excursion wardrobe")
	return nil, ""
}

func compatibleExcursionReactionNumbers(t *testing.T, characterID, placeID, costumeID string) []int32 {
	t.Helper()
	reactions, err := opmaster.FindExcursionGazeReactions(context.Background(), characterID, placeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, reaction := range reactions {
		if (len(reaction.GetUnlockCostumeIds()) == 0 || slices.Contains(reaction.GetUnlockCostumeIds(), costumeID)) &&
			!slices.Contains(reaction.GetUnlockNotCostumeIds(), costumeID) {
			return []int32{reaction.GetNumber()}
		}
	}
	return nil
}

func containsExcursionGift(gifts []*ptransaction.UserGift, itemID string) bool {
	for _, gift := range gifts {
		if gift.GetResourceType() == penum.ResourceType_ResourceType_Item && gift.GetResourceId() == itemID && gift.GetAmount() == 1 {
			return true
		}
	}
	return false
}

func activateMatchingMission(t *testing.T, missionType penum.MissionType, targetID string) *missionProgressExpectation {
	t.Helper()
	ctx := context.Background()
	masters, err := opmaster.FindAllMissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, master := range masters {
		if master.GetMissionType() != missionType ||
			len(master.GetTargetIds()) > 0 && !slices.Contains(master.GetTargetIds(), targetID) {
			continue
		}
		var currentTarget, maximum int64
		for _, progress := range master.GetProgresses() {
			threshold := progress.GetThreshold()
			if threshold <= 0 {
				continue
			}
			if currentTarget == 0 || threshold < currentTarget {
				currentTarget = threshold
			}
			if threshold > maximum {
				maximum = threshold
			}
		}
		if currentTarget == 0 {
			continue
		}
		if err := opuser.UpsertUserMission(ctx, testConfig.TestUserSuid, &ptransaction.UserMission{
			MissionId:        master.GetId(),
			CurrentThreshold: currentTarget,
			StatusType:       penum.MissionStatusType_MissionStatusType_Progress,
		}); err != nil {
			t.Fatal(err)
		}
		return &missionProgressExpectation{
			missionID: master.GetId(), maximum: maximum, currentTarget: currentTarget,
		}
	}
	t.Logf("current master data has no matching %s mission; mission post-image assertion is not applicable", missionType)
	return nil
}

func assertMissionAdvanced(
	t *testing.T,
	expectation *missionProgressExpectation,
	increment int64,
	updatedMissions []*ptransaction.UserMission,
) {
	t.Helper()
	if expectation == nil {
		return
	}
	stored, err := findUserMission(context.Background(), expectation.missionID)
	wantProgress := min(expectation.before+increment, expectation.maximum)
	if err != nil || stored.GetProgress() != wantProgress || stored.GetCurrentThreshold() != expectation.currentTarget || stored.GetLastUpdateTime() <= 0 {
		t.Fatalf(
			"mission %q persisted post-image = %v, err = %v; want progress=%d currentThreshold=%d",
			expectation.missionID, stored, err, wantProgress, expectation.currentTarget,
		)
	}
	returned := findUserMissionByID(updatedMissions, expectation.missionID)
	if returned == nil || !proto.Equal(returned, stored) {
		t.Fatalf("mission %q response post-image = %v, want stored %v", expectation.missionID, returned, stored)
	}
}
