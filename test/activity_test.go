package test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestActivityLessonScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)

	dbCtx := context.Background()
	progress, err := opuser.FindUserLessonProgress(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	// Make a full idle-reward window available without making the test wait.
	now := time.Now()
	progress.LastReceiveTime = now.Add(-13 * time.Hour).UnixMilli()
	progress.MaxRewardTime = now.Add(-time.Hour).UnixMilli()
	progress.RewardGoldAmount = 0
	progress.RewardManagerExpAmount = 0
	progress.RewardCardEnhanceItemAmount = 0
	if err := opuser.UpsertUserLessonProgress(dbCtx, testConfig.TestUserSuid, progress); err != nil {
		t.Fatal(err)
	}

	resourcesBefore := readActivityResourceTotal(t)
	activityClient := papi.NewActivityClient(testConfig.GrpcConn)
	receiveResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityReceiveLessonRewardResponse, error) {
		return activityClient.ReceiveLessonReward(ctx, &papi.Empty{})
	})
	assertActivityCommonResponse(t, receiveResponse.GetCommonResponse())
	if len(receiveResponse.GetRewards()) == 0 {
		t.Fatal("ReceiveLessonReward returned no accrued rewards")
	}
	if receiveResponse.GetProgress() == nil {
		t.Fatal("ReceiveLessonReward returned no lesson progress")
	}
	if receiveResponse.GetProgress().GetRewardGoldAmount() != 0 ||
		receiveResponse.GetProgress().GetRewardManagerExpAmount() != 0 ||
		receiveResponse.GetProgress().GetRewardCardEnhanceItemAmount() != 0 {
		t.Fatalf("ReceiveLessonReward did not clear accrued amounts: %v", receiveResponse.GetProgress())
	}
	if resourcesAfter := readActivityResourceTotal(t); resourcesAfter <= resourcesBefore {
		t.Fatalf("ReceiveLessonReward did not persist resources: before=%d after=%d", resourcesBefore, resourcesAfter)
	}

	promoteResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityPromoteLessonResponse, error) {
		return activityClient.PromoteLesson(ctx, &papi.ActivityPromoteLessonRequest{IsFree: true})
	})
	assertActivityCommonResponse(t, promoteResponse.GetCommonResponse())
	if promoteResponse.GetProgress() == nil {
		t.Fatal("PromoteLesson returned no lesson progress")
	}
	if promoteResponse.GetProgress().GetDailyPromoteCount() <= receiveResponse.GetProgress().GetDailyPromoteCount() {
		t.Fatalf("PromoteLesson did not increment the daily count: before=%d after=%d",
			receiveResponse.GetProgress().GetDailyPromoteCount(), promoteResponse.GetProgress().GetDailyPromoteCount())
	}
	if promoteResponse.GetProgress().GetLastReceiveTime() != receiveResponse.GetProgress().GetLastReceiveTime() ||
		promoteResponse.GetProgress().GetMaxRewardTime() != receiveResponse.GetProgress().GetMaxRewardTime() {
		t.Fatalf("PromoteLesson moved an idle-reward anchor without a complete pending step: before=%v after=%v",
			receiveResponse.GetProgress(), promoteResponse.GetProgress())
	}

	persisted, err := opuser.FindUserLessonProgress(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.GetDailyPromoteCount() != promoteResponse.GetProgress().GetDailyPromoteCount() {
		t.Fatalf("lesson progress was not persisted: db=%v response=%v", persisted, promoteResponse.GetProgress())
	}
	home := enterActivityHome(t)
	if home.GetLessonProgress() == nil || home.GetLessonInfo().GetLessonProgress() == nil {
		t.Fatal("Home.Enter omitted lesson progress")
	}
	if home.GetLessonProgress().GetDailyPromoteCount() != persisted.GetDailyPromoteCount() ||
		home.GetLessonInfo().GetLessonProgress().GetDailyPromoteCount() != persisted.GetDailyPromoteCount() {
		t.Fatalf("Home.Enter returned stale lesson progress: home=%v lessonInfo=%v db=%v",
			home.GetLessonProgress(), home.GetLessonInfo().GetLessonProgress(), persisted)
	}
}

func TestActivityBeginnerCampaignScenario(t *testing.T) {
	setupTestUser(t)

	home := enterActivityHome(t)
	if value, ok := activityCampaignEffectValue(
		home.GetLessonInfo().GetCampaignEffects(),
		penum.ActivityCampaignEffectType_ActivityCampaignEffectType_LessonExpUp,
	); !ok || value != 1000 {
		t.Fatalf("fresh Home.Enter omitted the beginner lesson campaign: %v", home.GetLessonInfo())
	}
	if value, ok := activityCampaignEffectValue(
		home.GetPromotionInfo().GetCampaignEffects(),
		penum.ActivityCampaignEffectType_ActivityCampaignEffectType_PromotionCardEnhanceItemUp,
	); !ok || value != 1000 {
		t.Fatalf("fresh Home.Enter omitted the beginner promotion campaign: %v", home.GetPromotionInfo())
	}

	dbCtx := context.Background()
	settingID, err := config.GetGameSettingId()
	if err != nil {
		t.Fatal(err)
	}
	setting, err := opmaster.FindSetting(dbCtx, settingID)
	if err != nil {
		t.Fatal(err)
	}
	stepMillis := int64(setting.GetSecondsPerActivityLessonStep()) * 1000
	if stepMillis <= 0 {
		t.Fatalf("invalid lesson step duration: %v", setting)
	}
	progress, err := opuser.FindUserLessonProgress(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	anchor := time.Now().Add(-5 * time.Duration(stepMillis) * time.Millisecond).UnixMilli()
	progress.LastReceiveTime = anchor
	progress.MaxRewardTime = time.Now().Add(12 * time.Hour).UnixMilli()
	if err := opuser.UpsertUserLessonProgress(dbCtx, testConfig.TestUserSuid, progress); err != nil {
		t.Fatal(err)
	}
	userBefore, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}

	activityClient := papi.NewActivityClient(testConfig.GrpcConn)
	response := activityCall(t, func(ctx context.Context) (*papi.ActivityReceiveLessonRewardResponse, error) {
		return activityClient.ReceiveLessonReward(ctx, &papi.Empty{})
	})
	assertActivityCommonResponse(t, response.GetCommonResponse())
	if value, ok := activityCampaignEffectValue(
		response.GetCampaignEffects(),
		penum.ActivityCampaignEffectType_ActivityCampaignEffectType_LessonExpUp,
	); !ok || value != 1000 {
		t.Fatalf("ReceiveLessonReward omitted the active beginner campaign: %v", response.GetCampaignEffects())
	}
	consumedSteps := (response.GetProgress().GetLastReceiveTime() - anchor) / stepMillis
	if consumedSteps <= 0 {
		t.Fatalf("ReceiveLessonReward consumed no elapsed beginner steps: %v", response.GetProgress())
	}
	wantManagerExp := consumedSteps * int64(progress.GetStepRewardManagerExp()) * 2
	var managerExpReward int64
	for _, reward := range response.GetRewards() {
		if reward.GetResourceType() == penum.ResourceType_ResourceType_ManagerExp {
			managerExpReward += reward.GetAmount()
		}
	}
	if managerExpReward != wantManagerExp {
		t.Fatalf("beginner lesson EXP campaign was not applied: got=%d want=%d", managerExpReward, wantManagerExp)
	}
	userAfter, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if userAfter.GetManagerExp()-userBefore.GetManagerExp() != wantManagerExp {
		t.Fatalf("beginner lesson EXP was not persisted: before=%d after=%d want-delta=%d",
			userBefore.GetManagerExp(), userAfter.GetManagerExp(), wantManagerExp)
	}
}

func TestActivityFanEventScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)

	dbCtx := context.Background()
	party := selectActivityParty(t)
	fanEvent := selectFanEvent(t)
	fanInfo, err := opuser.FindUserFanEventInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range fanInfo.GetFanEvents() {
		if candidate.GetId() == fanEvent.GetId() {
			candidate.IsUnlocked = false
			break
		}
	}
	if err := opuser.UpsertUserFanEventInfo(dbCtx, testConfig.TestUserSuid, fanInfo); err != nil {
		t.Fatal(err)
	}
	superModeItemID := findActivityItem(t, penum.ItemType_ItemType_ActivityFanEventSuperMode)
	superModeItem, err := opmaster.FindItem(dbCtx, superModeItemID)
	if err != nil {
		t.Fatal(err)
	}
	if superModeItem.GetValue() <= 0 {
		t.Fatalf("fan-event super-mode item has invalid value: %v", superModeItem)
	}
	superModeItemBefore := activityUserItemAmount(t, superModeItemID)
	totalBefore, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	resourcesBefore := readActivityResourceTotal(t)
	activityClient := papi.NewActivityClient(testConfig.GrpcConn)
	startResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityStartFanEventResponse, error) {
		return activityClient.StartFanEvent(ctx, &papi.ActivityStartFanEventRequest{
			ActivityFanEventId: fanEvent.GetId(),
			CharacterIds:       party.characterIDs,
			CostumeIds:         party.costumeIDs,
			HairIds:            party.hairIDs,
		})
	})
	assertActivityCommonResponse(t, startResponse.GetCommonResponse())
	progress := startResponse.GetProgress()
	if progress == nil || progress.GetActivityFanEventId() != fanEvent.GetId() || len(progress.GetCharacters()) != 3 {
		t.Fatalf("unexpected StartFanEvent progress: %v", progress)
	}
	assertActivityCharacters(t, party.characterIDs, true)
	persisted, err := opuser.FindUserFanEventRun(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.GetActivityFanEventId() != progress.GetActivityFanEventId() {
		t.Fatalf("fan-event run was not persisted: db=%v response=%v", persisted, progress)
	}
	home := enterActivityHome(t)
	if home.GetFanEventInfo().GetActiveFanEvent() == nil ||
		!activitySameStrings(home.GetFanEventInfo().GetActiveFanEvent().GetCharacterIds(), party.characterIDs) {
		t.Fatalf("Home.Enter omitted the active fan event: %v", home.GetFanEventInfo())
	}

	// A newly started event must not be settleable for repeatable fixed rewards.
	runBeforeEarlyFinish := proto.Clone(persisted).(*papi.FanEventProgress)
	infoBeforeEarlyFinish, err := opuser.FindUserFanEventInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	totalBeforeEarlyFinish, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	resourcesBeforeEarlyFinish := readActivityResourceTotal(t)
	earlyFinishCtx, earlyFinishCancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, earlyFinishErr := activityClient.FinishFanEvent(earlyFinishCtx, &papi.Empty{})
	earlyFinishCancel()
	if status.Code(earlyFinishErr) != codes.FailedPrecondition {
		t.Fatalf("FinishFanEvent before completion error=%v, want FailedPrecondition", earlyFinishErr)
	}
	runAfterEarlyFinish, err := opuser.FindUserFanEventRun(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(runBeforeEarlyFinish, runAfterEarlyFinish) {
		t.Fatalf("rejected FinishFanEvent mutated the run: before=%v after=%v", runBeforeEarlyFinish, runAfterEarlyFinish)
	}
	infoAfterEarlyFinish, err := opuser.FindUserFanEventInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(infoBeforeEarlyFinish, infoAfterEarlyFinish) {
		t.Fatalf("rejected FinishFanEvent mutated fan-event info: before=%v after=%v", infoBeforeEarlyFinish, infoAfterEarlyFinish)
	}
	totalAfterEarlyFinish, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(totalBeforeEarlyFinish, totalAfterEarlyFinish) {
		t.Fatalf("rejected FinishFanEvent mutated total count: before=%v after=%v", totalBeforeEarlyFinish, totalAfterEarlyFinish)
	}
	if resourcesAfter := readActivityResourceTotal(t); resourcesAfter != resourcesBeforeEarlyFinish {
		t.Fatalf("rejected FinishFanEvent mutated resources: before=%d after=%d", resourcesBeforeEarlyFinish, resourcesAfter)
	}
	assertActivityCharacters(t, party.characterIDs, true)

	loadResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityLoadFanEventResponse, error) {
		return activityClient.LoadFanEvent(ctx, &papi.Empty{})
	})
	assertActivityCommonResponse(t, loadResponse.GetCommonResponse())
	if loadResponse.GetProgress() == nil || loadResponse.GetProgress().GetActivityFanEventId() != fanEvent.GetId() {
		t.Fatalf("LoadFanEvent returned the wrong run: %v", loadResponse.GetProgress())
	}

	// Expose elapsed steps for Fetch/Save without sleeping.
	persisted.StartTime = time.Now().Add(-time.Hour).UnixMilli()
	for _, character := range persisted.GetCharacters() {
		character.FinishTime = time.Now().Add(time.Hour).UnixMilli()
		if character.MaxStep < 2 {
			character.MaxStep = 2
		}
	}
	if err := opuser.UpsertUserFanEventRun(dbCtx, testConfig.TestUserSuid, persisted); err != nil {
		t.Fatal(err)
	}
	steps := make([]int32, len(persisted.GetCharacters()))
	for index, character := range persisted.GetCharacters() {
		steps[index] = character.GetCompletedStep()
	}
	fetchResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityFetchFanEventStepsResponse, error) {
		return activityClient.FetchFanEventSteps(ctx, &papi.ActivityFetchFanEventStepsRequest{Steps: steps})
	})
	assertActivityCommonResponse(t, fetchResponse.GetCommonResponse())
	if len(fetchResponse.GetCharacterSteps()) != len(party.characterIDs) {
		t.Fatalf("FetchFanEventSteps returned %d character entries, want %d", len(fetchResponse.GetCharacterSteps()), len(party.characterIDs))
	}
	assertFanEventIssuedSteps(t, persisted, steps, fetchResponse.GetCharacterSteps(), 0)

	characterResults := fanEventResults(t, persisted, 1000, 0)
	saveResponse := activityCall(t, func(ctx context.Context) (*papi.ActivitySaveFanEventResponse, error) {
		return activityClient.SaveFanEvent(ctx, &papi.ActivitySaveFanEventRequest{CharacterResults: characterResults})
	})
	assertActivityCommonResponse(t, saveResponse.GetCommonResponse())
	persisted, err = opuser.FindUserFanEventRun(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, character := range persisted.GetCharacters() {
		if character.GetCompletedStep() == 0 || character.GetActivityPoint() == 0 {
			t.Fatalf("SaveFanEvent did not persist character progress: %v", character)
		}
	}

	activationResponse := activityCall(t, func(ctx context.Context) (*papi.ActivitySaveFanEventResponse, error) {
		return activityClient.SaveFanEvent(ctx, &papi.ActivitySaveFanEventRequest{
			CharacterResults: fanEventResults(t, persisted, 1000, 0),
			UseItemId:        superModeItemID,
		})
	})
	assertActivityCommonResponse(t, activationResponse.GetCommonResponse())
	if itemAfter := activityUserItemAmount(t, superModeItemID); itemAfter != superModeItemBefore-1 {
		t.Fatalf("SaveFanEvent consumed the wrong super-mode item amount: before=%d after=%d", superModeItemBefore, itemAfter)
	}
	persisted, err = opuser.FindUserFanEventRun(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.GetUsedSuperModeItem() {
		t.Fatal("SaveFanEvent did not persist super mode")
	}
	activationCursors := fanEventCursors(persisted)
	assertFanEventIssuedSteps(t, persisted, activationCursors, activationResponse.GetCharacterSteps(), superModeItem.GetValue())

	finishTimesBeforeReduction := make(map[string]int64, len(persisted.GetCharacters()))
	for _, character := range persisted.GetCharacters() {
		finishTimesBeforeReduction[character.GetCharacterId()] = character.GetFinishTime()
	}
	reduction := min(int64(1000), int64(superModeItem.GetValue()))
	superResponse := activityCall(t, func(ctx context.Context) (*papi.ActivitySaveFanEventResponse, error) {
		return activityClient.SaveFanEvent(ctx, &papi.ActivitySaveFanEventRequest{
			CharacterResults: fanEventResults(t, persisted, int64(1000)+int64(superModeItem.GetValue()), reduction),
		})
	})
	assertActivityCommonResponse(t, superResponse.GetCommonResponse())
	persisted, err = opuser.FindUserFanEventRun(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, character := range persisted.GetCharacters() {
		if got := finishTimesBeforeReduction[character.GetCharacterId()] - character.GetFinishTime(); got != reduction {
			t.Fatalf("super-mode reduction for %q=%d, want %d", character.GetCharacterId(), got, reduction)
		}
		if character.GetLastSavedActivityPoint() != character.GetActivityPoint() {
			t.Fatalf("SaveFanEvent did not update last-saved point for %q: %v", character.GetCharacterId(), character)
		}
	}
	assertFanEventIssuedSteps(t, persisted, fanEventCursors(persisted), superResponse.GetCharacterSteps(), superModeItem.GetValue())

	for _, character := range persisted.GetCharacters() {
		character.CompletedStep = character.GetMaxStep()
		character.FinishTime = time.Now().Add(-time.Minute).UnixMilli()
	}
	if err := opuser.UpsertUserFanEventRun(dbCtx, testConfig.TestUserSuid, persisted); err != nil {
		t.Fatal(err)
	}
	finishInfo, err := opuser.FindUserFanEventInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if finishInfo.GetActiveFanEvent() != nil {
		finishInfo.ActiveFanEvent.FinishTime = time.Now().Add(-time.Minute).UnixMilli()
		if err := opuser.UpsertUserFanEventInfo(dbCtx, testConfig.TestUserSuid, finishInfo); err != nil {
			t.Fatal(err)
		}
	}

	finishResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityFinishFanEventResponse, error) {
		return activityClient.FinishFanEvent(ctx, &papi.Empty{})
	})
	assertActivityCommonResponse(t, finishResponse.GetCommonResponse())
	if finishResponse.GetActivityPoint() <= 0 || len(finishResponse.GetRewards()) == 0 {
		t.Fatalf("FinishFanEvent returned no earned result: %v", finishResponse)
	}
	assertActivityRunDeleted(t, activityRunMissingFinder(opuser.FindUserFanEventRun))
	assertActivityCharacters(t, party.characterIDs, false)
	info, err := opuser.FindUserFanEventInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if info.GetActiveFanEvent() != nil {
		t.Fatalf("FinishFanEvent left an active Home summary: %v", info.GetActiveFanEvent())
	}
	totalAfter, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if totalAfter.GetActivityFanEventCount() <= totalBefore.GetActivityFanEventCount() {
		t.Fatalf("fan-event total count did not advance: before=%d after=%d",
			totalBefore.GetActivityFanEventCount(), totalAfter.GetActivityFanEventCount())
	}
	if resourcesAfter := readActivityResourceTotal(t); resourcesAfter <= resourcesBefore {
		t.Fatalf("FinishFanEvent did not persist rewards: before=%d after=%d", resourcesBefore, resourcesAfter)
	}

	rankingResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityGetFanEventRankingResponse, error) {
		return activityClient.GetFanEventRanking(ctx, &papi.ActivityGetFanEventRankingRequest{ActivityFanEventId: fanEvent.GetId()})
	})
	assertActivityCommonResponse(t, rankingResponse.GetCommonResponse())
	if rankingResponse.GetSelfRank() != 1 || len(rankingResponse.GetRankInfos()) != 1 {
		t.Fatalf("private-server fan-event ranking was not self-only: %v", rankingResponse)
	}
	if rankingResponse.GetSelfScore() <= 0 || rankingResponse.GetRankInfos()[0].GetScore() != rankingResponse.GetSelfScore() ||
		rankingResponse.GetRankInfos()[0].GetUserId() != testConfig.TestUserPublicId {
		t.Fatalf("fan-event self ranking is inconsistent: %v", rankingResponse)
	}
	if home = enterActivityHome(t); home.GetFanEventInfo().GetActiveFanEvent() != nil {
		t.Fatalf("Home.Enter still reports a finished fan event: %v", home.GetFanEventInfo().GetActiveFanEvent())
	}
}

func TestActivityPromotionScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)

	dbCtx := context.Background()
	party := selectActivityParty(t)
	promotion := selectPromotion(t)
	promotionInfo, err := opuser.FindUserPromotionInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	promotionInfo.CampaignEffects = []*papi.ActivityCampaignEffect{
		{
			Type:    penum.ActivityCampaignEffectType_ActivityCampaignEffectType_PromotionCardEnhanceItemUp,
			Value:   1000,
			EndTime: time.Now().Add(time.Hour).UnixMilli(),
		},
	}
	for _, candidate := range promotionInfo.GetPromotions() {
		if candidate.GetId() == promotion.GetId() {
			candidate.IsUnlocked = false
			break
		}
	}
	if err := opuser.UpsertUserPromotionInfo(dbCtx, testConfig.TestUserSuid, promotionInfo); err != nil {
		t.Fatal(err)
	}
	shortenItemID := findActivityItem(t,
		penum.ItemType_ItemType_ActivityPromotionShorten,
		penum.ItemType_ItemType_ActivityPromotionSpecialShorten,
	)
	itemBefore := activityUserItemAmount(t, shortenItemID)
	resourcesBefore := readActivityResourceTotal(t)
	fansBefore := make(map[string]int64, len(party.characters))
	for _, character := range party.characters {
		fansBefore[character.GetCharacterId()] = activityCharacterFanTotal(character)
	}
	totalBefore, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	activityClient := papi.NewActivityClient(testConfig.GrpcConn)
	startResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityStartPromotionResponse, error) {
		return activityClient.StartPromotion(ctx, &papi.ActivityStartPromotionRequest{
			ActivityPromotionId: promotion.GetId(),
			CharacterIds:        party.characterIDs,
			CostumeIds:          party.costumeIDs,
			HairIds:             party.hairIDs,
		})
	})
	assertActivityCommonResponse(t, startResponse.GetCommonResponse())
	progress := startResponse.GetProgress()
	if progress == nil || progress.GetActivityPromotionId() != promotion.GetId() || len(progress.GetCharacters()) != 3 {
		t.Fatalf("unexpected StartPromotion progress: %v", progress)
	}
	assertActivityCharacters(t, party.characterIDs, true)
	persisted, err := opuser.FindUserPromotionRun(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.GetActivityPromotionId() != progress.GetActivityPromotionId() {
		t.Fatalf("promotion run was not persisted: db=%v response=%v", persisted, progress)
	}
	home := enterActivityHome(t)
	if home.GetPromotionInfo().GetActivePromotion() == nil ||
		!activitySameStrings(home.GetPromotionInfo().GetActivePromotion().GetCharacterIds(), party.characterIDs) {
		t.Fatalf("Home.Enter omitted the active promotion: %v", home.GetPromotionInfo())
	}

	loadResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityLoadPromotionResponse, error) {
		return activityClient.LoadPromotion(ctx, &papi.Empty{})
	})
	assertActivityCommonResponse(t, loadResponse.GetCommonResponse())
	if loadResponse.GetProgress() == nil || loadResponse.GetProgress().GetActivityPromotionId() != promotion.GetId() {
		t.Fatalf("LoadPromotion returned the wrong run: %v", loadResponse.GetProgress())
	}

	fetchResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityFetchPromotionStepsResponse, error) {
		return activityClient.FetchPromotionSteps(ctx, &papi.ActivityFetchPromotionStepsRequest{Step: progress.GetCompletedStep()})
	})
	assertActivityCommonResponse(t, fetchResponse.GetCommonResponse())
	for _, step := range fetchResponse.GetSteps() {
		if step.GetStep() <= progress.GetCompletedStep() {
			t.Fatalf("FetchPromotionSteps returned a stale step: current=%d step=%v", progress.GetCompletedStep(), step)
		}
	}

	finishTimeBeforeItem := progress.GetFinishTime()
	useResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityUsePromotionItemResponse, error) {
		return activityClient.UsePromotionItem(ctx, &papi.ActivityUsePromotionItemRequest{ItemId: shortenItemID, Amount: 1})
	})
	assertActivityCommonResponse(t, useResponse.GetCommonResponse())
	if useResponse.GetProgress() == nil || useResponse.GetProgress().GetFinishTime() >= finishTimeBeforeItem {
		t.Fatalf("UsePromotionItem did not shorten the run: before=%d response=%v", finishTimeBeforeItem, useResponse.GetProgress())
	}
	if itemAfter := activityUserItemAmount(t, shortenItemID); itemAfter != itemBefore-1 {
		t.Fatalf("UsePromotionItem consumed the wrong amount: before=%d after=%d", itemBefore, itemAfter)
	}

	// Give ReceivePromotionReward several elapsed steps without sleeping.
	persisted, err = opuser.FindUserPromotionRun(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	persisted.StartTime = time.Now().Add(-time.Hour).UnixMilli()
	if persisted.GetFinishTime() <= time.Now().UnixMilli() {
		persisted.FinishTime = time.Now().Add(time.Hour).UnixMilli()
	}
	if err := opuser.UpsertUserPromotionRun(dbCtx, testConfig.TestUserSuid, persisted); err != nil {
		t.Fatal(err)
	}
	rewardLoadResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityLoadPromotionResponse, error) {
		return activityClient.LoadPromotion(ctx, &papi.Empty{})
	})
	if rewardLoadResponse.GetProgress().GetCompletedStep() < 20 || len(rewardLoadResponse.GetProgress().GetRewards()) == 0 {
		t.Fatalf("LoadPromotion did not persist elapsed multi-step rewards: %v", rewardLoadResponse.GetProgress())
	}
	if want := int64(rewardLoadResponse.GetProgress().GetCompletedStep()) *
		int64(rewardLoadResponse.GetProgress().GetStepRewardCardEnhanceItem()) * 2; int64(rewardLoadResponse.GetProgress().GetRewardCardEnhanceItemAmount()) != want {
		t.Fatalf("promotion card-enhance campaign was not applied: got=%d want=%d",
			rewardLoadResponse.GetProgress().GetRewardCardEnhanceItemAmount(), want)
	}
	dropTotals := make(map[activityRewardKey]int64)
	for _, reward := range rewardLoadResponse.GetProgress().GetRewards() {
		dropTotals[activityRewardKey{resourceType: reward.GetResourceType(), resourceID: reward.GetResourceId()}] += reward.GetAmount()
	}
	receiveResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityReceivePromotionRewardResponse, error) {
		return activityClient.ReceivePromotionReward(ctx, &papi.Empty{})
	})
	assertActivityCommonResponse(t, receiveResponse.GetCommonResponse())
	if receiveResponse.GetProgress() == nil || receiveResponse.GetCompletedStep() == 0 || len(receiveResponse.GetRewards()) == 0 {
		t.Fatalf("ReceivePromotionReward returned no elapsed reward: %v", receiveResponse)
	}
	persisted, err = opuser.FindUserPromotionRun(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.GetCompletedStep() != receiveResponse.GetProgress().GetCompletedStep() {
		t.Fatalf("ReceivePromotionReward did not persist progress: db=%v response=%v", persisted, receiveResponse.GetProgress())
	}
	if len(persisted.GetRewards()) != 0 {
		t.Fatalf("ReceivePromotionReward did not clear settled drops: %v", persisted.GetRewards())
	}
	runState, err := opuser.FindUserPromotionRunState(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(receiveResponse.GetCompletedStep()) % 20; runState.FanRewardStepRemainder != want {
		t.Fatalf("promotion fan reward remainder was not persisted: got=%d want=%d",
			runState.FanRewardStepRemainder, want)
	}
	if !runState.AccountingInitialized || runState.LastSettledCompletedStep != int64(receiveResponse.GetCompletedStep()) {
		t.Fatalf("promotion settlement cursor was not persisted: %v", runState)
	}
	receivedTotals := make(map[activityRewardKey]int64)
	for _, reward := range receiveResponse.GetRewards() {
		receivedTotals[activityRewardKey{resourceType: reward.GetResourceType(), resourceID: reward.GetResourceId()}] += reward.GetAmount()
	}
	for key, amount := range dropTotals {
		if receivedTotals[key] < amount {
			t.Fatalf("ReceivePromotionReward omitted multi-step drop %+v: got=%d want-at-least=%d", key, receivedTotals[key], amount)
		}
	}
	expectedFanGain := int64(receiveResponse.GetCompletedStep()) / 20
	for _, character := range assertActivityCharacters(t, party.characterIDs, true) {
		remaining := int64(promotion.GetMaxFanAmount()) - fansBefore[character.GetCharacterId()]
		want := fansBefore[character.GetCharacterId()] + min(expectedFanGain, max(int64(0), remaining))
		if got := activityCharacterFanTotal(character); got != want {
			t.Fatalf("promotion fan gain is inconsistent for %s: got=%d want=%d", character.GetCharacterId(), got, want)
		}
	}
	levelProgress := receiveResponse.GetActivityLevelProgress()
	if levelProgress.GetBefore().GetLevel() == levelProgress.GetMaxActivityLevel() &&
		(levelProgress.GetBefore().GetNextLevelLocked() || levelProgress.GetAfter().GetNextLevelLocked()) {
		t.Fatalf("max-level promotion was reported as next-level locked: %v", levelProgress)
	}

	// Complete the remaining duration without waiting and then settle the run.
	persisted.StartTime = time.Now().Add(-24 * time.Hour).UnixMilli()
	persisted.FinishTime = time.Now().Add(-time.Minute).UnixMilli()
	if err := opuser.UpsertUserPromotionRun(dbCtx, testConfig.TestUserSuid, persisted); err != nil {
		t.Fatal(err)
	}
	promotionInfo, err = opuser.FindUserPromotionInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if promotionInfo.GetActivePromotion() != nil {
		promotionInfo.ActivePromotion.FinishTime = persisted.GetFinishTime()
		if err := opuser.UpsertUserPromotionInfo(dbCtx, testConfig.TestUserSuid, promotionInfo); err != nil {
			t.Fatal(err)
		}
	}
	finishCalledAt := time.Now().UnixMilli()
	finishResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityFinishPromotionResponse, error) {
		return activityClient.FinishPromotion(ctx, &papi.Empty{})
	})
	assertActivityCommonResponse(t, finishResponse.GetCommonResponse())
	if finishResponse.GetTotalSteps() < receiveResponse.GetCompletedStep() || len(finishResponse.GetCharacters()) != 3 {
		t.Fatalf("FinishPromotion returned an inconsistent result: %v", finishResponse)
	}
	assertActivityRunDeleted(t, activityRunMissingFinder(opuser.FindUserPromotionRun))
	for _, character := range assertActivityCharacters(t, party.characterIDs, false) {
		if character.GetStaminaUpdatedTime() < finishCalledAt {
			t.Fatalf("FinishPromotion did not timestamp character %s: got=%d minimum=%d",
				character.GetCharacterId(), character.GetStaminaUpdatedTime(), finishCalledAt)
		}
		remaining := int64(promotion.GetMaxFanAmount()) - fansBefore[character.GetCharacterId()]
		want := fansBefore[character.GetCharacterId()] + min(int64(finishResponse.GetTotalSteps())/20, max(int64(0), remaining))
		if got := activityCharacterFanTotal(character); got != want {
			t.Fatalf("FinishPromotion lost a partial fan-reward interval for %s: got=%d want=%d",
				character.GetCharacterId(), got, want)
		}
	}
	promotionInfo, err = opuser.FindUserPromotionInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if promotionInfo.GetActivePromotion() != nil {
		t.Fatalf("FinishPromotion left an active Home summary: %v", promotionInfo.GetActivePromotion())
	}
	totalAfter, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if totalAfter.GetActivityPromotionMinutes() <= totalBefore.GetActivityPromotionMinutes() {
		t.Fatalf("promotion minutes did not advance: before=%d after=%d",
			totalBefore.GetActivityPromotionMinutes(), totalAfter.GetActivityPromotionMinutes())
	}
	if resourcesAfter := readActivityResourceTotal(t); resourcesAfter <= resourcesBefore-1 {
		t.Fatalf("promotion rewards were not persisted: before=%d after=%d", resourcesBefore, resourcesAfter)
	}
	if home = enterActivityHome(t); home.GetPromotionInfo().GetActivePromotion() != nil {
		t.Fatalf("Home.Enter still reports a finished promotion: %v", home.GetPromotionInfo().GetActivePromotion())
	}
}

func TestActivityRefreshScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)

	dbCtx := context.Background()
	party := selectActivityParty(t)
	selection := selectRefresh(t)
	refreshInfo, err := opuser.FindUserRefreshInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	liveCampaignEnd := now.Add(time.Hour).UnixMilli()
	refreshInfo.CampaignEffects = []*papi.ActivityCampaignEffect{
		{
			Type:    penum.ActivityCampaignEffectType_ActivityCampaignEffectType_RefreshGoldConsumptionDown,
			Value:   500,
			EndTime: liveCampaignEnd,
		},
		{
			Type:    penum.ActivityCampaignEffectType_ActivityCampaignEffectType_RefreshGoldConsumptionDown,
			Value:   1,
			EndTime: now.Add(-time.Hour).UnixMilli(),
		},
	}
	if err := opuser.UpsertUserRefreshInfo(dbCtx, testConfig.TestUserSuid, refreshInfo); err != nil {
		t.Fatal(err)
	}
	expectedGoldCost := int64(selection.level.GetRequiredGold()) * 500 / 1000
	homeBeforeStart := enterActivityHome(t)
	var projectedGold int32 = -1
	for _, refresh := range homeBeforeStart.GetRefreshInfo().GetRefreshes() {
		if refresh.GetId() != selection.refresh.GetId() {
			continue
		}
		for _, level := range refresh.GetLevels() {
			if level.GetLevel() == selection.level.GetLevel() {
				projectedGold = level.GetRequiredGold()
				break
			}
		}
	}
	if projectedGold != int32(expectedGoldCost) {
		t.Fatalf("Home.Enter did not project the live refresh discount: got=%d want=%d",
			projectedGold, expectedGoldCost)
	}
	persistedRefreshInfo, err := opuser.FindUserRefreshInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, refresh := range persistedRefreshInfo.GetRefreshes() {
		if refresh.GetId() != selection.refresh.GetId() {
			continue
		}
		for _, level := range refresh.GetLevels() {
			if level.GetLevel() == selection.level.GetLevel() && level.GetRequiredGold() != selection.level.GetRequiredGold() {
				t.Fatalf("Home.Enter persisted a discounted refresh price: got=%d base=%d",
					level.GetRequiredGold(), selection.level.GetRequiredGold())
			}
		}
	}
	shortenItemID := findActivityItem(t, penum.ItemType_ItemType_ActivityRefreshShorten)
	itemBefore := activityUserItemAmount(t, shortenItemID)
	for _, character := range party.characters {
		character.ActivityStamina = 0
		character.StaminaUpdatedTime = time.Now().UnixMilli()
		if err := opuser.UpsertUserCharacter(dbCtx, testConfig.TestUserSuid, character); err != nil {
			t.Fatal(err)
		}
	}
	goldBefore := activityUserGoldAmount(t)
	totalBefore, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	activityClient := papi.NewActivityClient(testConfig.GrpcConn)
	startResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityStartRefreshResponse, error) {
		return activityClient.StartRefresh(ctx, &papi.ActivityStartRefreshRequest{
			ActivityRefreshId:    selection.refresh.GetId(),
			ActivityRefreshLevel: selection.level.GetLevel(),
			CharacterIds:         party.characterIDs,
			CostumeIds:           party.costumeIDs,
			HairIds:              party.hairIDs,
		})
	})
	assertActivityCommonResponse(t, startResponse.GetCommonResponse())
	progress := startResponse.GetProgress()
	if progress == nil || progress.GetActivityRefreshId() != selection.refresh.GetId() ||
		progress.GetActivityRefreshLevel() != selection.level.GetLevel() || len(progress.GetCharacters()) != 3 {
		t.Fatalf("unexpected StartRefresh progress: %v", progress)
	}
	if goldAfter := activityUserGoldAmount(t); goldAfter != goldBefore-expectedGoldCost {
		t.Fatalf("StartRefresh consumed the wrong gold amount: before=%d cost=%d after=%d",
			goldBefore, expectedGoldCost, goldAfter)
	}
	refreshInfo, err = opuser.FindUserRefreshInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(refreshInfo.GetCampaignEffects()) != 1 || refreshInfo.GetCampaignEffects()[0].GetEndTime() != liveCampaignEnd {
		t.Fatalf("StartRefresh did not preserve only the live campaign effect: %v", refreshInfo.GetCampaignEffects())
	}
	assertActivityCharacters(t, party.characterIDs, true)
	persisted, err := opuser.FindUserRefreshRun(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.GetActivityRefreshId() != progress.GetActivityRefreshId() {
		t.Fatalf("refresh run was not persisted: db=%v response=%v", persisted, progress)
	}
	home := enterActivityHome(t)
	if home.GetRefreshInfo().GetActiveRefresh() == nil ||
		!activitySameStrings(home.GetRefreshInfo().GetActiveRefresh().GetCharacterIds(), party.characterIDs) {
		t.Fatalf("Home.Enter omitted the active refresh: %v", home.GetRefreshInfo())
	}

	loadResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityLoadRefreshResponse, error) {
		return activityClient.LoadRefresh(ctx, &papi.Empty{})
	})
	assertActivityCommonResponse(t, loadResponse.GetCommonResponse())
	if loadResponse.GetProgress() == nil || loadResponse.GetProgress().GetActivityRefreshId() != selection.refresh.GetId() {
		t.Fatalf("LoadRefresh returned the wrong run: %v", loadResponse.GetProgress())
	}

	finishTimeBeforeItem := progress.GetFinishTime()
	useResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityUseRefreshItemResponse, error) {
		return activityClient.UseRefreshItem(ctx, &papi.ActivityUseRefreshItemRequest{ItemId: shortenItemID, Amount: 1})
	})
	assertActivityCommonResponse(t, useResponse.GetCommonResponse())
	if useResponse.GetProgress() == nil || useResponse.GetProgress().GetFinishTime() >= finishTimeBeforeItem {
		t.Fatalf("UseRefreshItem did not shorten the run: before=%d response=%v", finishTimeBeforeItem, useResponse.GetProgress())
	}
	if itemAfter := activityUserItemAmount(t, shortenItemID); itemAfter != itemBefore-1 {
		t.Fatalf("UseRefreshItem consumed the wrong amount: before=%d after=%d", itemBefore, itemAfter)
	}

	persisted, err = opuser.FindUserRefreshRun(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	persisted.StartTime = time.Now().Add(-time.Hour).UnixMilli()
	persisted.FinishTime = time.Now().Add(-time.Minute).UnixMilli()
	if err := opuser.UpsertUserRefreshRun(dbCtx, testConfig.TestUserSuid, persisted); err != nil {
		t.Fatal(err)
	}
	refreshInfo, err = opuser.FindUserRefreshInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if refreshInfo.GetActiveRefresh() != nil {
		refreshInfo.ActiveRefresh.FinishTime = persisted.GetFinishTime()
		if err := opuser.UpsertUserRefreshInfo(dbCtx, testConfig.TestUserSuid, refreshInfo); err != nil {
			t.Fatal(err)
		}
	}
	finishResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityFinishRefreshResponse, error) {
		return activityClient.FinishRefresh(ctx, &papi.Empty{})
	})
	assertActivityCommonResponse(t, finishResponse.GetCommonResponse())
	if len(finishResponse.GetCharacters()) != 3 {
		t.Fatalf("FinishRefresh returned %d characters, want 3", len(finishResponse.GetCharacters()))
	}
	assertActivityRunDeleted(t, activityRunMissingFinder(opuser.FindUserRefreshRun))
	charactersAfter := assertActivityCharacters(t, party.characterIDs, false)
	for _, character := range charactersAfter {
		if character.GetActivityStamina() <= 0 {
			t.Fatalf("FinishRefresh did not recover character stamina: %v", character)
		}
		activityLevel, err := opmaster.FindCharacterActivityLevel(dbCtx, int64(character.GetActivityLevel()))
		if err != nil {
			t.Fatal(err)
		}
		if character.GetActivityStamina() > activityLevel.GetMaxStamina() {
			t.Fatalf("FinishRefresh exceeded max stamina: character=%v max=%d", character, activityLevel.GetMaxStamina())
		}
	}
	refreshInfo, err = opuser.FindUserRefreshInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if refreshInfo.GetActiveRefresh() != nil {
		t.Fatalf("FinishRefresh left an active Home summary: %v", refreshInfo.GetActiveRefresh())
	}
	totalAfter, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if totalAfter.GetActivityRefreshCount() <= totalBefore.GetActivityRefreshCount() {
		t.Fatalf("refresh total count did not advance: before=%d after=%d",
			totalBefore.GetActivityRefreshCount(), totalAfter.GetActivityRefreshCount())
	}
	if home = enterActivityHome(t); home.GetRefreshInfo().GetActiveRefresh() != nil {
		t.Fatalf("Home.Enter still reports a finished refresh: %v", home.GetRefreshInfo().GetActiveRefresh())
	}
}

func TestActivityRefreshCancellationScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)

	dbCtx := context.Background()
	party := selectActivityParty(t)
	selection := selectRefresh(t)
	refreshInfo, err := opuser.FindUserRefreshInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, refresh := range refreshInfo.GetRefreshes() {
		if refresh.GetId() != selection.refresh.GetId() {
			continue
		}
		refresh.IsUnlocked = false
		for _, level := range refresh.GetLevels() {
			if level.GetLevel() == selection.level.GetLevel() {
				level.IsUnlocked = false
			}
		}
	}
	if err := opuser.UpsertUserRefreshInfo(dbCtx, testConfig.TestUserSuid, refreshInfo); err != nil {
		t.Fatal(err)
	}
	for _, character := range party.characters {
		character.ActivityStamina = 0
		character.StaminaUpdatedTime = time.Now().UnixMilli()
		if err := opuser.UpsertUserCharacter(dbCtx, testConfig.TestUserSuid, character); err != nil {
			t.Fatal(err)
		}
	}
	totalBefore, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	refreshExpBefore := selection.refresh.GetExp()
	activityClient := papi.NewActivityClient(testConfig.GrpcConn)
	startResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityStartRefreshResponse, error) {
		return activityClient.StartRefresh(ctx, &papi.ActivityStartRefreshRequest{
			ActivityRefreshId:    selection.refresh.GetId(),
			ActivityRefreshLevel: selection.level.GetLevel(),
			CharacterIds:         party.characterIDs,
			CostumeIds:           party.costumeIDs,
			HairIds:              party.hairIDs,
		})
	})
	if startResponse.GetProgress() == nil || startResponse.GetProgress().GetFinishTime() <= time.Now().UnixMilli() {
		t.Fatalf("StartRefresh did not create a future run: %v", startResponse.GetProgress())
	}
	staminaBeforeCancel := make(map[string]int32, len(party.characterIDs))
	for _, character := range assertActivityCharacters(t, party.characterIDs, true) {
		staminaBeforeCancel[character.GetCharacterId()] = character.GetActivityStamina()
	}

	finishCalledAt := time.Now().UnixMilli()
	finishResponse := activityCall(t, func(ctx context.Context) (*papi.ActivityFinishRefreshResponse, error) {
		return activityClient.FinishRefresh(ctx, &papi.Empty{})
	})
	assertActivityCommonResponse(t, finishResponse.GetCommonResponse())
	if finishResponse.GetCommonResponse().GetUpdatedData().GetTotalCount() != nil {
		t.Fatalf("cancelled refresh unexpectedly updated total count: %v", finishResponse.GetCommonResponse().GetUpdatedData().GetTotalCount())
	}
	levelProgress := finishResponse.GetActivityLevelProgress()
	if levelProgress.GetBefore().GetExp() != levelProgress.GetAfter().GetExp() ||
		levelProgress.GetBefore().GetLevel() != levelProgress.GetAfter().GetLevel() {
		t.Fatalf("cancelled refresh unexpectedly granted activity progress: %v", levelProgress)
	}
	assertActivityRunDeleted(t, activityRunMissingFinder(opuser.FindUserRefreshRun))
	for _, character := range assertActivityCharacters(t, party.characterIDs, false) {
		if character.GetActivityStamina() != staminaBeforeCancel[character.GetCharacterId()] {
			t.Fatalf("cancelled refresh recovered stamina for %s: before=%d after=%d",
				character.GetCharacterId(), staminaBeforeCancel[character.GetCharacterId()], character.GetActivityStamina())
		}
		if character.GetStaminaUpdatedTime() < finishCalledAt {
			t.Fatalf("cancelled refresh did not timestamp character %s", character.GetCharacterId())
		}
	}
	refreshInfo, err = opuser.FindUserRefreshInfo(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if refreshInfo.GetActiveRefresh() != nil {
		t.Fatalf("cancelled refresh left an active summary: %v", refreshInfo.GetActiveRefresh())
	}
	for _, refresh := range refreshInfo.GetRefreshes() {
		if refresh.GetId() == selection.refresh.GetId() && refresh.GetExp() != refreshExpBefore {
			t.Fatalf("cancelled refresh changed exp: before=%d after=%d", refreshExpBefore, refresh.GetExp())
		}
	}
	totalAfter, err := opuser.FindUserTotalCount(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if totalAfter.GetActivityRefreshCount() != totalBefore.GetActivityRefreshCount() {
		t.Fatalf("cancelled refresh changed total count: before=%d after=%d",
			totalBefore.GetActivityRefreshCount(), totalAfter.GetActivityRefreshCount())
	}
}

func activityCall[T any](t *testing.T, call func(context.Context) (T, error)) T {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		var zero T
		t.Fatal(err)
		return zero
	}
	defer cancel()
	response, err := call(ctx)
	if err != nil {
		var zero T
		t.Fatal(err)
		return zero
	}
	return response
}

func assertActivityCommonResponse(t *testing.T, response *papi.CommonResponse) {
	t.Helper()
	if response == nil || response.GetUpdatedData() == nil || response.GetDeletedData() == nil {
		t.Fatalf("activity API returned an incomplete common response: %v", response)
	}
}

func enterActivityHome(t *testing.T) *papi.HomeEnterResponse {
	t.Helper()
	client := papi.NewHomeClient(testConfig.GrpcConn)
	return activityCall(t, func(ctx context.Context) (*papi.HomeEnterResponse, error) {
		return client.Enter(ctx, &papi.HomeEnterRequest{})
	})
}

func readActivityResourceTotal(t *testing.T) int64 {
	t.Helper()
	ctx := context.Background()
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	total := user.GetManagerExp()
	items, err := opuser.FindAllUserItems(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		total += item.GetAmount()
	}
	points, err := opuser.FindAllUserPoints(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range points {
		total += point.GetAmount()
	}
	accessories, err := opuser.FindAllUserAccessories(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, accessory := range accessories {
		total += accessory.GetAmount()
	}
	return total
}

type activityParty struct {
	characterIDs []string
	costumeIDs   []string
	hairIDs      []string
	characters   []*ptransaction.UserCharacter
}

type activityRewardKey struct {
	resourceType penum.ResourceType
	resourceID   string
}

func activityCharacterFanTotal(character *ptransaction.UserCharacter) int64 {
	return int64(character.GetLightFanAmount()) + int64(character.GetMiddleFanAmount()) + int64(character.GetHeavyFanAmount())
}

func selectActivityParty(t *testing.T) activityParty {
	t.Helper()
	ctx := context.Background()
	characters, err := opuser.FindAllUserCharacters(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	ownedCostumes, err := opuser.FindAllUserCostumes(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	ownedHairs, err := opuser.FindAllUserHairs(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	costumeSet := make(map[string]struct{}, len(ownedCostumes))
	for _, costume := range ownedCostumes {
		costumeSet[costume.GetCostumeId()] = struct{}{}
	}
	hairSet := make(map[string]struct{}, len(ownedHairs))
	for _, hair := range ownedHairs {
		hairSet[hair.GetHairId()] = struct{}{}
	}
	masterCostumes, err := opmaster.FindAllCostumes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(masterCostumes, func(i, j int) bool { return masterCostumes[i].GetId() < masterCostumes[j].GetId() })
	type wardrobe struct{ costumeID, hairID string }
	wardrobes := make(map[string]wardrobe)
	for _, costume := range masterCostumes {
		if costume.GetCharacterId() == "" || costume.GetDefaultHairId() == "" || costume.GetIsUserUnavailable() {
			continue
		}
		if _, ok := costumeSet[costume.GetId()]; !ok {
			continue
		}
		if _, ok := hairSet[costume.GetDefaultHairId()]; !ok {
			continue
		}
		if _, exists := wardrobes[costume.GetCharacterId()]; !exists {
			wardrobes[costume.GetCharacterId()] = wardrobe{costumeID: costume.GetId(), hairID: costume.GetDefaultHairId()}
		}
	}
	sort.Slice(characters, func(i, j int) bool {
		if characters[i].GetActivityStamina() == characters[j].GetActivityStamina() {
			return characters[i].GetCharacterId() < characters[j].GetCharacterId()
		}
		return characters[i].GetActivityStamina() > characters[j].GetActivityStamina()
	})
	party := activityParty{}
	for _, character := range characters {
		wardrobe, ok := wardrobes[character.GetCharacterId()]
		if !ok || character.GetInActivity() {
			continue
		}
		party.characterIDs = append(party.characterIDs, character.GetCharacterId())
		party.costumeIDs = append(party.costumeIDs, wardrobe.costumeID)
		party.hairIDs = append(party.hairIDs, wardrobe.hairID)
		party.characters = append(party.characters, proto.Clone(character).(*ptransaction.UserCharacter))
		if len(party.characterIDs) == 3 {
			return party
		}
	}
	t.Fatalf("configured test user has only %d activity-ready characters", len(party.characterIDs))
	return activityParty{}
}

func selectFanEvent(t *testing.T) *papi.FanEvent {
	t.Helper()
	info, err := opuser.FindUserFanEventInfo(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(info.FanEvents, func(i, j int) bool { return info.FanEvents[i].GetId() < info.FanEvents[j].GetId() })
	for _, fanEvent := range info.GetFanEvents() {
		if fanEvent.GetIsUnlocked() && fanEvent.GetStepConsumptionStamina() > 0 && fanEvent.GetFixedRewardId() != "" {
			return proto.Clone(fanEvent).(*papi.FanEvent)
		}
	}
	t.Fatalf("no usable unlocked fan event was found: %v", info)
	return nil
}

func fanEventResults(
	t *testing.T,
	progress *papi.FanEventProgress,
	pointMultiplierPermil int64,
	reductionMilliSeconds int64,
) []*papi.FanEventCharacterResult {
	t.Helper()
	results := make([]*papi.FanEventCharacterResult, 0, len(progress.GetCharacters()))
	for _, activeCharacter := range progress.GetCharacters() {
		userCharacter, err := opuser.FindUserCharacter(context.Background(), testConfig.TestUserSuid, activeCharacter.GetCharacterId())
		if err != nil {
			t.Fatal(err)
		}
		activityLevel, err := opmaster.FindCharacterActivityLevel(context.Background(), int64(userCharacter.GetActivityLevel()))
		if err != nil {
			t.Fatal(err)
		}
		point := int64(activityLevel.GetBaseActivityPoint()) * int64(progress.GetStepActivityPointCoefficientPermil()) / 1000
		point = point * pointMultiplierPermil / 1000
		if point < 1 {
			point = 1
		}
		results = append(results, &papi.FanEventCharacterResult{
			CharacterId: activeCharacter.GetCharacterId(),
			StepResults: []*papi.FanEventStepResult{{
				Step:                  activeCharacter.GetCompletedStep() + 1,
				ActivityPoint:         int32(point),
				ReductionMilliSeconds: reductionMilliSeconds,
			}},
		})
	}
	return results
}

func fanEventCursors(progress *papi.FanEventProgress) []int32 {
	cursors := make([]int32, 0, len(progress.GetCharacters()))
	for _, character := range progress.GetCharacters() {
		cursors = append(cursors, character.GetCompletedStep())
	}
	return cursors
}

func assertFanEventIssuedSteps(
	t *testing.T,
	progress *papi.FanEventProgress,
	cursors []int32,
	characterSteps []*papi.FanEventCharacterSteps,
	superEffectValue int32,
) {
	t.Helper()
	if len(characterSteps) != len(progress.GetCharacters()) {
		t.Fatalf("issued fan-event character steps=%d, want %d", len(characterSteps), len(progress.GetCharacters()))
	}
	for index, activeCharacter := range progress.GetCharacters() {
		issued := characterSteps[index]
		if issued.GetCharacterId() != activeCharacter.GetCharacterId() {
			t.Fatalf("issued fan-event character=%q, want %q", issued.GetCharacterId(), activeCharacter.GetCharacterId())
		}
		if cursors[index] < activeCharacter.GetMaxStep() && len(issued.GetSteps()) == 0 {
			t.Fatalf("fan-event character %q received no upcoming step data", issued.GetCharacterId())
		}
		for stepIndex, step := range issued.GetSteps() {
			wantStep := cursors[index] + int32(stepIndex) + 1
			if step.GetStep() != wantStep {
				t.Fatalf("issued fan-event step=%d, want %d", step.GetStep(), wantStep)
			}
			var foundSuperEffect bool
			for _, effect := range step.GetItemEffects() {
				if effect.GetType() == penum.ItemType_ItemType_ActivityFanEventSuperMode {
					foundSuperEffect = true
					if effect.GetValue() != superEffectValue {
						t.Fatalf("super-mode effect value=%d, want %d", effect.GetValue(), superEffectValue)
					}
				}
			}
			if foundSuperEffect != (superEffectValue > 0) {
				t.Fatalf("issued super-mode effect=%v, want %v: %v", foundSuperEffect, superEffectValue > 0, step)
			}
		}
	}
}

func selectPromotion(t *testing.T) *papi.Promotion {
	t.Helper()
	info, err := opuser.FindUserPromotionInfo(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(info.Promotions, func(i, j int) bool { return info.Promotions[i].GetId() < info.Promotions[j].GetId() })
	for _, promotion := range info.GetPromotions() {
		if promotion.GetIsUnlocked() && promotion.GetStepConsumptionStamina() > 0 && promotion.GetMultiStepRewardId() != "" {
			return proto.Clone(promotion).(*papi.Promotion)
		}
	}
	t.Fatalf("no usable unlocked promotion was found: %v", info)
	return nil
}

type refreshSelection struct {
	refresh *papi.Refresh
	level   *papi.RefreshLevel
}

func selectRefresh(t *testing.T) refreshSelection {
	t.Helper()
	info, err := opuser.FindUserRefreshInfo(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(info.Refreshes, func(i, j int) bool { return info.Refreshes[i].GetId() < info.Refreshes[j].GetId() })
	for _, refresh := range info.GetRefreshes() {
		if !refresh.GetIsUnlocked() || refresh.GetRequiredMinutes() <= 0 {
			continue
		}
		levels := append([]*papi.RefreshLevel(nil), refresh.GetLevels()...)
		sort.Slice(levels, func(i, j int) bool { return levels[i].GetLevel() < levels[j].GetLevel() })
		for _, level := range levels {
			if level.GetIsUnlocked() && level.GetRequiredGold() > 0 && level.GetRecoveryAmount() > 0 {
				return refreshSelection{
					refresh: proto.Clone(refresh).(*papi.Refresh),
					level:   proto.Clone(level).(*papi.RefreshLevel),
				}
			}
		}
	}
	t.Fatalf("no usable unlocked refresh was found: %v", info)
	return refreshSelection{}
}

func findActivityItem(t *testing.T, itemTypes ...penum.ItemType) string {
	t.Helper()
	typeSet := make(map[penum.ItemType]struct{}, len(itemTypes))
	for _, itemType := range itemTypes {
		typeSet[itemType] = struct{}{}
	}
	ctx := context.Background()
	items, err := opmaster.FindAllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].GetId() < items[j].GetId() })
	for _, item := range items {
		if _, ok := typeSet[item.GetType()]; !ok {
			continue
		}
		if amount := activityUserItemAmount(t, item.GetId()); amount > 0 {
			return item.GetId()
		}
	}
	t.Fatalf("configured test user owns no item with types %v", itemTypes)
	return ""
}

func activityUserItemAmount(t *testing.T, itemID string) int64 {
	t.Helper()
	items, err := opuser.FindUserItems(context.Background(), testConfig.TestUserSuid, itemID)
	if err != nil {
		t.Fatal(err)
	}
	var amount int64
	now := time.Now().UnixMilli()
	for _, item := range items {
		if item.GetExpiredTime() == 0 || item.GetExpiredTime() > now {
			amount += item.GetAmount()
		}
	}
	return amount
}

func activityUserGoldAmount(t *testing.T) int64 {
	t.Helper()
	gold, err := opuser.FindUserPoint(context.Background(), testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil {
		t.Fatal(err)
	}
	return gold.GetAmount()
}

func assertActivityCharacters(t *testing.T, characterIDs []string, inActivity bool) []*ptransaction.UserCharacter {
	t.Helper()
	characters := make([]*ptransaction.UserCharacter, 0, len(characterIDs))
	for _, characterID := range characterIDs {
		character, err := opuser.FindUserCharacter(context.Background(), testConfig.TestUserSuid, characterID)
		if err != nil {
			t.Fatal(err)
		}
		if character.GetInActivity() != inActivity {
			t.Fatalf("character %q inActivity=%v, want %v", characterID, character.GetInActivity(), inActivity)
		}
		characters = append(characters, character)
	}
	return characters
}

func assertActivityRunDeleted(t *testing.T, find func(context.Context, string) (proto.Message, error)) {
	t.Helper()
	if _, err := find(context.Background(), testConfig.TestUserSuid); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("finished activity run still exists: %v", err)
	}
}

func activitySameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func activityCampaignEffectValue(
	effects []*papi.ActivityCampaignEffect,
	effectType penum.ActivityCampaignEffectType,
) (int32, bool) {
	for _, effect := range effects {
		if effect.GetType() == effectType {
			return effect.GetValue(), true
		}
	}
	return 0, false
}

func activityRunMissingFinder[T proto.Message](find func(context.Context, string) (T, error)) func(context.Context, string) (proto.Message, error) {
	return func(ctx context.Context, suid string) (proto.Message, error) {
		return find(ctx, suid)
	}
}
