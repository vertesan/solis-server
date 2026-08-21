package test

import (
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	cardDailyMissionID       = "ms-daily-phase-003_card_level_up_count-160"
	cardLevelMissionID       = "ms-weekly-phase-003_card_level_up_count-160"
	cardSkillMissionID       = "ms-weekly-phase-003_card_skill_level_up_count-160"
	accessoryNormalMissionID = "ms-normal-accessory_enhance_count"
	accessoryDailyMissionID  = "ms-daily-phase-003_accessory_enhance_count-160"
	accessoryWeeklyMissionID = "ms-weekly-phase-003_accessory_enhance_count-160"
	weeklyClearMissionID     = "ms-weekly-phase-003_mission_clear_count-160-present_add"
)

func TestAccessoryCardScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	testCardProgression(t)
	testCardSupportProgression(t)
	testAccessoryProgression(t)
}

func testCardProgression(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	card, targetLevel, unlockedStoryIds := findCardStoryEnhanceTarget(t)
	upsertMissionFixture(t, cardDailyMissionID, 1, 0)
	upsertMissionFixture(t, cardLevelMissionID, 10, 9)
	upsertMissionFixture(t, weeklyClearMissionID, 6, 0)
	for _, storyId := range unlockedStoryIds {
		if err := opuser.DeleteUserStory(ctx, testConfig.TestUserSuid, storyId); err != nil {
			t.Fatalf("failed preparing locked card story %q: %v", storyId, err)
		}
	}

	cardClient := papi.NewCardClient(testConfig.GrpcConn)
	targetLevelMaster, err := opmaster.FindCardLevel(ctx, targetLevel)
	if err != nil {
		t.Fatal(err)
	}
	beforeEnhanceItem := userItemAmount(t, "item-card_enhance")
	expectedEnhanceCost := targetLevelMaster.RequiredExp - card.TotalExp
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	enhanceResponse, err := cardClient.Enhance(grpcCtx, &papi.CardEnhanceRequest{CardId: card.CardId, Level: targetLevel})
	cancel()
	if err != nil {
		t.Fatalf("Card.Enhance failed: %v", err)
	}
	if got := userItemAmount(t, "item-card_enhance"); got != beforeEnhanceItem-expectedEnhanceCost {
		t.Fatalf("Card.Enhance cost mismatch: got %d, want %d", got, beforeEnhanceItem-expectedEnhanceCost)
	}
	if len(enhanceResponse.GetCommonResponse().GetUpdatedData().GetCards()) != 1 ||
		enhanceResponse.GetCommonResponse().GetUpdatedData().GetCards()[0].GetBaseLevel() != targetLevel {
		t.Fatalf("Card.Enhance returned wrong post-image: %v", enhanceResponse)
	}
	expectedMissionProgress := cappedMissionProgress(t, cardLevelMissionID, 9, int64(targetLevel-card.BaseLevel))
	missionUpdates := enhanceResponse.GetCommonResponse().GetUpdatedData().GetMissions()
	assertMissionState(t, missionUpdates, cardDailyMissionID, 1, penum.MissionStatusType_MissionStatusType_NotReceived)
	assertMissionState(t, missionUpdates, cardLevelMissionID, expectedMissionProgress, penum.MissionStatusType_MissionStatusType_Clear)
	assertMissionState(t, missionUpdates, weeklyClearMissionID, 1, penum.MissionStatusType_MissionStatusType_Progress)
	assertPersistedMissionState(t, cardDailyMissionID, 1, penum.MissionStatusType_MissionStatusType_NotReceived)
	assertPersistedMissionState(t, cardLevelMissionID, expectedMissionProgress, penum.MissionStatusType_MissionStatusType_Clear)
	assertPersistedMissionState(t, weeklyClearMissionID, 1, penum.MissionStatusType_MissionStatusType_Progress)
	assertUnlockedStories(t, enhanceResponse.GetCommonResponse().GetUpdatedData().GetStories(), unlockedStoryIds)
	userStories, err := opuser.FindAllUserStories(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	assertUnlockedStories(t, userStories, unlockedStoryIds)
	// A regression request must fail without consuming another item.
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = cardClient.Enhance(grpcCtx, &papi.CardEnhanceRequest{CardId: card.CardId, Level: targetLevel})
	cancel()
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("Card.Enhance regression returned %v, want FailedPrecondition", err)
	}
	if got := userItemAmount(t, "item-card_enhance"); got != beforeEnhanceItem-expectedEnhanceCost {
		t.Fatalf("rejected Card.Enhance changed resources: got %d", got)
	}

	card, err = opuser.FindUserCard(ctx, testConfig.TestUserSuid, card.CardId)
	if err != nil {
		t.Fatal(err)
	}
	targetRarity := card.Rarity + 1
	targetRarityMaster, err := opmaster.FindCardRarity(ctx, int64(targetRarity))
	if err != nil {
		t.Fatal(err)
	}
	beforeLimitItem := userItemAmount(t, "item-card_limit_break")
	expectedLimitCost := int64(targetRarityMaster.RequiredExp - card.RarityTotalExp)
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	limitResponse, err := cardClient.LimitBreak(grpcCtx, &papi.CardLimitBreakRequest{CardId: card.CardId, Rarity: targetRarity})
	cancel()
	if err != nil {
		t.Fatalf("Card.LimitBreak failed: %v", err)
	}
	if got := userItemAmount(t, "item-card_limit_break"); got != beforeLimitItem-expectedLimitCost {
		t.Fatalf("Card.LimitBreak cost mismatch: got %d, want %d", got, beforeLimitItem-expectedLimitCost)
	}
	limitCard := limitResponse.GetCommonResponse().GetUpdatedData().GetCards()[0]
	if limitCard.GetRarity() != targetRarity {
		t.Fatalf("Card.LimitBreak returned wrong post-image: %v", limitResponse)
	}
	if limitCard.GetDisplayType() != card.GetDisplayType() {
		t.Fatalf("Card.LimitBreak changed user-selected display type: before=%v after=%v", card.GetDisplayType(), limitCard.GetDisplayType())
	}
}

func testCardSupportProgression(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	var card *ptransaction.UserCard
	var masterCard *pmaster.Card
	for _, candidate := range cards {
		if candidate.Supported {
			continue
		}
		master, findErr := opmaster.FindCard(ctx, candidate.CardId)
		if findErr == nil && master.SkillId1 != "" && master.LiveAbilityId != "" {
			card, masterCard = candidate, master
			break
		}
	}
	if card == nil {
		t.Fatal("no card can exercise support skill and live ability enhancement")
	}
	supports, err := opuser.FindAllUserCardSupports(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	var supportNumber int32 = 1
	occupied := make(map[int32]struct{}, len(supports))
	for _, support := range supports {
		occupied[support.Number] = struct{}{}
	}
	for ; supportNumber <= user.CardSupportMaxNumber; supportNumber++ {
		if _, exists := occupied[supportNumber]; !exists {
			break
		}
	}
	if supportNumber > user.CardSupportMaxNumber {
		t.Fatal("no unoccupied card support slot is available")
	}
	cardClient := papi.NewCardClient(testConfig.GrpcConn)
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = cardClient.SetSupport(grpcCtx, &papi.CardSetSupportRequest{
		Number: user.CardSupportMaxNumber + 1,
		CardId: card.CardId,
	})
	cancel()
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Card.SetSupport out-of-range slot returned %v, want InvalidArgument", err)
	}
	afterRejectedSupports, err := opuser.FindAllUserCardSupports(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	afterRejectedCard, err := opuser.FindUserCard(ctx, testConfig.TestUserSuid, card.CardId)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterRejectedSupports) != len(supports) || afterRejectedCard.Supported {
		t.Fatalf("rejected Card.SetSupport changed user state: supports=%d card=%v", len(afterRejectedSupports), afterRejectedCard)
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	supportResponse, err := cardClient.SetSupport(grpcCtx, &papi.CardSetSupportRequest{Number: supportNumber, CardId: card.CardId})
	cancel()
	if err != nil {
		t.Fatalf("Card.SetSupport failed: %v", err)
	}
	supportedCard := supportResponse.GetCommonResponse().GetUpdatedData().GetCards()[0]
	if !supportedCard.Supported || supportedCard.Level != user.CardSupportLevel {
		t.Fatalf("Card.SetSupport did not project support level: %v", supportedCard)
	}

	skill, err := opmaster.FindSkill(ctx, supportedCard.SkillId1)
	if err != nil {
		t.Fatal(err)
	}
	skillCost := nextSkillCost(t, skill, supportedCard.SkillLevel1)
	upsertMissionFixture(t, cardSkillMissionID, 1, 0)
	upsertMissionFixture(t, weeklyClearMissionID, 6, 0)
	beforeSkillItem := userItemAmount(t, "item-card_skill")
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	skillResponse, err := cardClient.SkillEnhance(grpcCtx, &papi.CardSkillEnhanceRequest{CardId: card.CardId, SkillId: supportedCard.SkillId1})
	cancel()
	if err != nil {
		t.Fatalf("Card.SkillEnhance failed: %v", err)
	}
	if got := userItemAmount(t, "item-card_skill"); got != beforeSkillItem-int64(skillCost) {
		t.Fatalf("Card.SkillEnhance cost mismatch: got %d, want %d", got, beforeSkillItem-int64(skillCost))
	}
	if skillResponse.GetCommonResponse().GetUpdatedData().GetCards()[0].GetSkillLevel1() != supportedCard.SkillLevel1+1 {
		t.Fatalf("Card.SkillEnhance returned wrong post-image: %v", skillResponse)
	}
	missionUpdates := skillResponse.GetCommonResponse().GetUpdatedData().GetMissions()
	assertMissionState(t, missionUpdates, cardSkillMissionID, 1, penum.MissionStatusType_MissionStatusType_Clear)
	assertMissionState(t, missionUpdates, weeklyClearMissionID, 1, penum.MissionStatusType_MissionStatusType_Progress)
	assertPersistedMissionState(t, cardSkillMissionID, 1, penum.MissionStatusType_MissionStatusType_Clear)
	assertPersistedMissionState(t, weeklyClearMissionID, 1, penum.MissionStatusType_MissionStatusType_Progress)

	ability, err := opmaster.FindLiveAbility(ctx, masterCard.LiveAbilityId)
	if err != nil {
		t.Fatal(err)
	}
	abilityCost := nextLiveAbilityCost(t, ability, supportedCard.LiveAbilityLevel)
	beforeAbilityItem := userItemAmount(t, "item-card_ability")
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	abilityResponse, err := cardClient.LiveAbilityEnhance(grpcCtx, &papi.CardLiveAbilityEnhanceRequest{CardId: card.CardId, AbilityId: masterCard.LiveAbilityId})
	cancel()
	if err != nil {
		t.Fatalf("Card.LiveAbilityEnhance failed: %v", err)
	}
	if got := userItemAmount(t, "item-card_ability"); got != beforeAbilityItem-int64(abilityCost) {
		t.Fatalf("Card.LiveAbilityEnhance cost mismatch: got %d, want %d", got, beforeAbilityItem-int64(abilityCost))
	}
	if abilityResponse.GetCommonResponse().GetUpdatedData().GetCards()[0].GetLiveAbilityLevel() != supportedCard.LiveAbilityLevel+1 {
		t.Fatalf("Card.LiveAbilityEnhance returned wrong post-image: %v", abilityResponse)
	}
}

func testAccessoryProgression(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	client := papi.NewAccessoryClient(testConfig.GrpcConn)
	phaseOneMaster, err := opmaster.FindAccessory(ctx, "ac-1-dance-22")
	if err != nil {
		t.Fatal(err)
	}
	phaseZeroMaster, err := opmaster.FindAccessoryByClassificationAndPhase(ctx, phaseOneMaster, phaseOneMaster.LimitBreakPhase-1)
	if err != nil {
		t.Fatal(err)
	}
	if phaseZeroMaster.Id != "ac-1-dance-21" {
		t.Fatalf("phase-one generic accessory predecessor mismatch: got %q, want %q", phaseZeroMaster.Id, "ac-1-dance-21")
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		t.Fatal(err)
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		t.Fatal(err)
	}
	targetMaster, err := opmaster.FindAccessory(ctx, "ac-2-stamina-21")
	if err != nil {
		t.Fatal(err)
	}
	materialMaster, err := opmaster.FindAccessoryEnhancementMaterial(ctx, targetMaster)
	if err != nil {
		t.Fatal(err)
	}
	if materialMaster.Id != "ac-2-stamina-20" {
		t.Fatalf("enhancement predecessor mismatch: got %q, want %q", materialMaster.Id, "ac-2-stamina-20")
	}
	beforeTarget := userAccessoryAmount(t, targetMaster.Id)
	beforeMaterial := userAccessoryAmount(t, materialMaster.Id)
	beforeGold := userGoldAmount(t)
	upsertMissionFixture(t, accessoryNormalMissionID, 10, 0)
	upsertMissionFixture(t, accessoryDailyMissionID, 1, 0)
	upsertMissionFixture(t, accessoryWeeklyMissionID, 10, 9)
	upsertMissionFixture(t, weeklyClearMissionID, 6, 0)
	enhancement, err := opmaster.FindAccessoryEnhancement(ctx, targetMaster.Rarity)
	if err != nil {
		t.Fatal(err)
	}
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Enhance(grpcCtx, &papi.AccessoryEnhanceRequest{
		AccessoryId: targetMaster.Id, Count: 1,
		MaterialType: penum.AccessoryEnhanceMaterialType_AccessoryEnhanceMaterialType_ExcludeAllDeck,
	})
	cancel()
	if err != nil {
		t.Fatalf("Accessory.Enhance failed: %v", err)
	}
	if userAccessoryAmount(t, targetMaster.Id) != beforeTarget+1 ||
		userAccessoryAmount(t, materialMaster.Id) != beforeMaterial-int64(setting.AccessoryEnhancesRequiredMaterialAmount) ||
		userGoldAmount(t) != beforeGold-enhancement.RequiredGold {
		t.Fatalf("Accessory.Enhance persisted incorrect resource deltas: %v", response)
	}
	missionUpdates := response.GetCommonResponse().GetUpdatedData().GetMissions()
	missionIncrease := int64(setting.AccessoryEnhancesRequiredMaterialAmount)
	assertMissionState(t, missionUpdates, accessoryNormalMissionID, missionIncrease, penum.MissionStatusType_MissionStatusType_Progress)
	assertMissionState(t, missionUpdates, accessoryDailyMissionID, 1, penum.MissionStatusType_MissionStatusType_NotReceived)
	assertMissionState(t, missionUpdates, accessoryWeeklyMissionID, 9+missionIncrease, penum.MissionStatusType_MissionStatusType_Clear)
	assertMissionState(t, missionUpdates, weeklyClearMissionID, 1, penum.MissionStatusType_MissionStatusType_Progress)
	assertPersistedMissionState(t, accessoryNormalMissionID, missionIncrease, penum.MissionStatusType_MissionStatusType_Progress)
	assertPersistedMissionState(t, accessoryDailyMissionID, 1, penum.MissionStatusType_MissionStatusType_NotReceived)
	assertPersistedMissionState(t, accessoryWeeklyMissionID, 9+missionIncrease, penum.MissionStatusType_MissionStatusType_Clear)
	assertPersistedMissionState(t, weeklyClearMissionID, 1, penum.MissionStatusType_MissionStatusType_Progress)

	limitMaster, err := opmaster.FindAccessory(ctx, "ac-2-stamina-32")
	if err != nil {
		t.Fatal(err)
	}
	previousMaster, err := opmaster.FindAccessoryByClassificationAndPhase(ctx, limitMaster, limitMaster.LimitBreakPhase-1)
	if err != nil {
		t.Fatal(err)
	}
	if previousMaster.Id != "ac-2-stamina-31" {
		t.Fatalf("generic accessory predecessor mismatch: got %q, want %q", previousMaster.Id, "ac-2-stamina-31")
	}

	// Limit-break phases must only be minted through LimitBreak, even if the
	// account owns enough same-class enhancement material.
	beforeRejectedTarget := userAccessoryAmount(t, limitMaster.Id)
	beforeRejectedGold := userGoldAmount(t)
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Enhance(grpcCtx, &papi.AccessoryEnhanceRequest{
		AccessoryId:  limitMaster.Id,
		Count:        1,
		MaterialType: penum.AccessoryEnhanceMaterialType_AccessoryEnhanceMaterialType_ExcludeAllDeck,
	})
	cancel()
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("Accessory.Enhance accepted limit-break phase: %v", err)
	}
	if userAccessoryAmount(t, limitMaster.Id) != beforeRejectedTarget || userGoldAmount(t) != beforeRejectedGold {
		t.Fatal("rejected limit-phase Accessory.Enhance changed user resources")
	}

	// Persisted deck state is authoritative. Deliberately send false request
	// hints and verify the server still replaces an equipped predecessor.
	deckPositions, err := opuser.FindAllUserDeckPositions(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(deckPositions) == 0 {
		t.Fatal("fixed test user has no deck position for Accessory.LimitBreak")
	}
	equippedPosition := deckPositions[0]
	equippedPosition.Part1AccessoryId = previousMaster.Id
	if err := opuser.UpsertUserDeckPosition(ctx, testConfig.TestUserSuid, equippedPosition); err != nil {
		t.Fatal(err)
	}
	beforeLimitTarget := userAccessoryAmount(t, limitMaster.Id)
	beforePrevious := userAccessoryAmount(t, previousMaster.Id)
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	limitResponse, err := client.LimitBreak(grpcCtx, &papi.AccessoryLimitBreakRequest{AccessoryId: limitMaster.Id})
	cancel()
	if err != nil {
		t.Fatalf("Accessory.LimitBreak failed: %v", err)
	}
	if userAccessoryAmount(t, limitMaster.Id) != beforeLimitTarget+1 || userAccessoryAmount(t, previousMaster.Id) != beforePrevious-1 {
		t.Fatalf("Accessory.LimitBreak persisted incorrect accessory deltas: %v", limitResponse)
	}
	positionsAfter, err := opuser.FindUserDeckPosition(ctx, testConfig.TestUserSuid, equippedPosition.Number, equippedPosition.Position)
	if err != nil {
		t.Fatal(err)
	}
	if len(positionsAfter) != 1 || positionsAfter[0].Part1AccessoryId != limitMaster.Id {
		t.Fatalf("Accessory.LimitBreak trusted false usage flags: positions=%v", positionsAfter)
	}
	foundDeckDelta := false
	for _, position := range limitResponse.GetCommonResponse().GetUpdatedData().GetDeckPositions() {
		if position.Number == equippedPosition.Number && position.Position == equippedPosition.Position && position.Part1AccessoryId == limitMaster.Id {
			foundDeckDelta = true
			break
		}
	}
	if !foundDeckDelta {
		t.Fatalf("Accessory.LimitBreak omitted authoritative deck post-image: %v", limitResponse)
	}
}

func findCardStoryEnhanceTarget(t *testing.T) (*ptransaction.UserCard, int32, []string) {
	t.Helper()
	ctx := context.Background()
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	managerLevel, err := opmaster.FindManagerLevel(ctx, user.ManagerLevel)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, card := range cards {
		if card.Supported || card.BaseLevel <= 0 || card.Rarity >= 10 {
			continue
		}
		masterCard, err := opmaster.FindCard(ctx, card.CardId)
		if err != nil || len(masterCard.Stories) == 0 {
			continue
		}
		rarity, err := opmaster.FindCardRarity(ctx, int64(card.Rarity))
		if err != nil {
			continue
		}
		maxLevel := rarity.LevelLimit
		if managerLevel.MaxCardLevel < maxLevel {
			maxLevel = managerLevel.MaxCardLevel
		}
		releases, err := opmaster.FindCardLevelReleases(ctx, masterCard.CardLevelReleaseId, maxLevel)
		if err != nil {
			continue
		}
		currentStoryCount := int32(0)
		for _, release := range releases {
			for _, target := range release.Targets {
				if target.Type != penum.CardLevelReleaseType_CardLevelReleaseType_Story {
					continue
				}
				if release.Level <= card.BaseLevel {
					currentStoryCount = target.Number
					continue
				}
				if target.Number <= currentStoryCount || target.Number > int32(len(masterCard.Stories)) {
					continue
				}
				storyIds := make([]string, 0, target.Number-currentStoryCount)
				for _, story := range masterCard.Stories[currentStoryCount:target.Number] {
					storyIds = append(storyIds, story.StoryId)
				}
				return card, release.Level, storyIds
			}
		}
	}
	t.Fatal("no card can cross a reachable story unlock threshold")
	return nil, 0, nil
}

func assertUnlockedStories(t *testing.T, stories []*ptransaction.UserStory, expectedIds []string) {
	t.Helper()
	actual := make(map[string]struct{}, len(stories))
	for _, story := range stories {
		actual[story.StoryId] = struct{}{}
	}
	for _, expectedId := range expectedIds {
		if _, exists := actual[expectedId]; !exists {
			t.Fatalf("card story %q was not unlocked; stories=%v", expectedId, stories)
		}
	}
}

func nextSkillCost(t *testing.T, skill *pmaster.Skill, current int32) int32 {
	t.Helper()
	for _, level := range skill.Levels {
		if level.Level == current+1 {
			return level.RequiredItemAmount
		}
	}
	t.Fatalf("skill %q has no level after %d", skill.Id, current)
	return 0
}

func nextLiveAbilityCost(t *testing.T, ability *pmaster.LiveAbility, current int32) int32 {
	t.Helper()
	for _, level := range ability.Levels {
		if level.Level == current+1 {
			return level.RequiredItemAmount
		}
	}
	t.Fatalf("live ability %q has no level after %d", ability.Id, current)
	return 0
}

func userAccessoryAmount(t *testing.T, accessoryID string) int64 {
	t.Helper()
	accessory, err := opuser.FindUserAccessory(context.Background(), testConfig.TestUserSuid, accessoryID)
	if err != nil {
		t.Fatalf("failed reading accessory %q: %v", accessoryID, err)
	}
	return accessory.Amount
}

func userGoldAmount(t *testing.T) int64 {
	t.Helper()
	point, err := opuser.FindUserPoint(context.Background(), testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil {
		t.Fatalf("failed reading user gold: %v", err)
	}
	return point.Amount
}

func upsertMissionFixture(t *testing.T, missionID string, threshold, progress int64) {
	t.Helper()
	masters, err := opmaster.FindMissionsByIds(context.Background(), []string{missionID})
	if err != nil {
		t.Fatalf("failed reading mission master %q: %v", missionID, err)
	}
	if len(masters) != 1 {
		t.Fatalf("mission master %q count=%d, want 1", missionID, len(masters))
	}
	validThreshold := false
	for _, missionProgress := range masters[0].Progresses {
		if missionProgress.Threshold == threshold {
			validThreshold = true
			break
		}
	}
	if !validThreshold {
		t.Fatalf("mission %q has no threshold %d", missionID, threshold)
	}
	if err := opuser.UpsertUserMission(context.Background(), testConfig.TestUserSuid, &ptransaction.UserMission{
		MissionId: missionID, CurrentThreshold: threshold, Progress: progress,
		StatusType: penum.MissionStatusType_MissionStatusType_Progress,
	}); err != nil {
		t.Fatalf("failed preparing mission %q: %v", missionID, err)
	}
}

func cappedMissionProgress(t *testing.T, missionID string, progress, increment int64) int64 {
	t.Helper()
	masters, err := opmaster.FindMissionsByIds(context.Background(), []string{missionID})
	if err != nil || len(masters) != 1 {
		t.Fatalf("failed reading mission master %q: count=%d err=%v", missionID, len(masters), err)
	}
	maximum := int64(0)
	for _, missionProgress := range masters[0].Progresses {
		if missionProgress.Threshold > maximum {
			maximum = missionProgress.Threshold
		}
	}
	if progress+increment > maximum {
		return maximum
	}
	return progress + increment
}

func assertPersistedMissionState(t *testing.T, missionID string, progress int64, statusType penum.MissionStatusType) {
	t.Helper()
	missions, err := opuser.FindAllUserMissions(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatalf("failed reading persisted missions: %v", err)
	}
	assertMissionState(t, missions, missionID, progress, statusType)
}

func assertMissionState(
	t *testing.T,
	missions []*ptransaction.UserMission,
	missionID string,
	progress int64,
	statusType penum.MissionStatusType,
) {
	t.Helper()
	for _, mission := range missions {
		if mission.MissionId != missionID {
			continue
		}
		if mission.Progress != progress || mission.StatusType != statusType || mission.LastUpdateTime <= 0 {
			t.Fatalf(
				"mission %q state=(progress=%d status=%s time=%d), want (progress=%d status=%s positive time)",
				missionID,
				mission.Progress,
				mission.StatusType,
				mission.LastUpdateTime,
				progress,
				statusType,
			)
		}
		return
	}
	t.Fatalf("mission %q missing from updates: %v", missionID, missions)
}
