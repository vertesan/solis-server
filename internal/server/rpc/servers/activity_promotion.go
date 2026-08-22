package servers

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ActivityServer) StartPromotion(ctx context.Context, req *papi.ActivityStartPromotionRequest) (*papi.ActivityStartPromotionResponse, error) {
	if req == nil || req.GetActivityPromotionId() == "" {
		return nil, viber.InvalidArgumentf("activityPromotionId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityStartPromotionResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		info, err := opuser.FindUserPromotionInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		now := time.Now().UnixMilli()
		if err := viber.ReconcilePromotionInfo(ctx, suid, now, info); err != nil {
			return nil, err
		}
		if info.GetActivePromotion() != nil {
			return nil, viber.FailedPreconditionf("a promotion is already active")
		}
		if _, err := opuser.FindUserPromotionRun(ctx, suid); err == nil {
			return nil, viber.FailedPreconditionf("a promotion is already active")
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		promotion := activityPromotionByID(info, req.GetActivityPromotionId())
		if promotion == nil || !promotion.GetIsUnlocked() {
			return nil, viber.FailedPreconditionf("promotion %q is not unlocked", req.GetActivityPromotionId())
		}
		characters, lineup, err := viber.ValidateActivityLineup(
			ctx, suid, promotion.GetActivityPerformanceId(), req.GetCharacterIds(), req.GetCostumeIds(), req.GetHairIds(), now,
		)
		if err != nil {
			return nil, err
		}
		settingID, err := config.GetGameSettingId()
		if err != nil {
			return nil, err
		}
		setting, err := opmaster.FindSetting(ctx, settingID)
		if err != nil {
			return nil, err
		}
		secondsPerStep := setting.GetSecondsPerActivityPromotionStep()
		if secondsPerStep <= 0 || promotion.GetStepConsumptionStamina() <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "promotion timing is not configured")
		}
		var availableSteps int64
		for _, character := range characters {
			if character.GetInActivity() {
				return nil, viber.FailedPreconditionf("character %q is already in an activity", character.GetCharacterId())
			}
			if character.GetActivityStamina() < promotion.GetRequiredStamina() {
				return nil, viber.FailedPreconditionf("character %q has insufficient activity stamina", character.GetCharacterId())
			}
			availableSteps += int64(character.GetActivityStamina() / promotion.GetStepConsumptionStamina())
		}
		if availableSteps <= 0 {
			return nil, viber.FailedPreconditionf("the selected characters have insufficient activity stamina")
		}
		if availableSteps > math.MaxInt32 {
			availableSteps = math.MaxInt32
		}
		if availableSteps > (math.MaxInt64-now)/(int64(secondsPerStep)*1000) {
			return nil, status.Error(codes.FailedPrecondition, "promotion finish time overflows")
		}
		finishTime := now + availableSteps*int64(secondsPerStep)*1000
		managerExp, activityExp, characterExp := promotionStepExperience(promotion.GetStepRewardGold())
		progress := &papi.PromotionProgress{
			ActivityPromotionId:            promotion.GetId(),
			ActivityPromotionName:          promotion.GetName(),
			ActivityPromotionSubName:       promotion.GetSubName(),
			ActivityPromotionLevel:         promotion.GetLevel(),
			Characters:                     lineup,
			StartTime:                      now,
			FinishTime:                     finishTime,
			StepConsumptionStamina:         promotion.GetStepConsumptionStamina(),
			StepRewardGold:                 promotion.GetStepRewardGold(),
			StepRewardManagerExp:           managerExp,
			StepRewardActivityExp:          activityExp,
			StepRewardCharacterActivityExp: characterExp,
			StepAdditionalRewardId:         promotion.GetStepAdditionalRewardId(),
			MultiStepRewardId:              promotion.GetMultiStepRewardId(),
			StepRewardCardEnhanceItem:      promotion.GetStepRewardCardEnhanceItem(),
			CampaignEffects:                info.GetCampaignEffects(),
			ActivityPerformanceId:          promotion.GetActivityPerformanceId(),
			SubGenre:                       promotion.GetSubGenre(),
			MultiStepCampaignDropRewardIds: promotion.GetMultiStepCampaignDropRewardIds(),
		}
		updated := &papi.UpdatedData{Characters: characters}
		for _, character := range characters {
			character.InActivity = true
			if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
				return nil, err
			}
			missions, err := viber.AdvanceUserMissions(
				ctx, suid, penum.MissionType_MissionType_CharacterStartActivityPromotionCount, 1, character.GetCharacterId(),
			)
			if err != nil {
				return nil, err
			}
			updated.Missions = append(updated.Missions, missions...)
		}
		if err := opuser.UpsertUserPromotionRun(ctx, suid, progress); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserPromotionRunState(ctx, &umodel.UserPromotionRunState{
			SUID:                  suid,
			AccountingInitialized: true,
		}); err != nil {
			return nil, err
		}
		info.ActivePromotion = &papi.ActivePromotion{
			CharacterIds:    append([]string(nil), req.GetCharacterIds()...),
			Name:            promotion.GetName(),
			SubName:         promotion.GetSubName(),
			Level:           promotion.GetLevel(),
			FinishTime:      finishTime,
			CampaignEffects: info.GetCampaignEffects(),
		}
		if err := opuser.UpsertUserPromotionInfo(ctx, suid, info); err != nil {
			return nil, err
		}
		response = &papi.ActivityStartPromotionResponse{
			Progress:       progress,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *ActivityServer) LoadPromotion(ctx context.Context, _ *papi.Empty) (*papi.ActivityLoadPromotionResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityLoadPromotionResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserPromotionRun(ctx, suid)
		if err != nil {
			return nil, activityRunError("promotion", err)
		}
		if _, err := advancePromotion(ctx, suid, progress, time.Now().UnixMilli()); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserPromotionRun(ctx, suid, progress); err != nil {
			return nil, err
		}
		response = &papi.ActivityLoadPromotionResponse{
			Progress:       progress,
			CommonResponse: viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *ActivityServer) UsePromotionItem(ctx context.Context, req *papi.ActivityUsePromotionItemRequest) (*papi.ActivityUsePromotionItemResponse, error) {
	if req == nil || req.GetItemId() == "" || req.GetAmount() <= 0 {
		return nil, viber.InvalidArgumentf("a positive promotion item amount is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityUsePromotionItemResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserPromotionRun(ctx, suid)
		if err != nil {
			return nil, activityRunError("promotion", err)
		}
		item, err := opmaster.FindItem(ctx, req.GetItemId())
		if err != nil {
			return nil, err
		}
		switch item.GetType() {
		case penum.ItemType_ItemType_ActivityPromotionShorten,
			penum.ItemType_ItemType_ActivityPromotionSpecialShorten:
		default:
			return nil, viber.InvalidArgumentf("item %q cannot shorten promotions", req.GetItemId())
		}
		if item.GetValue() <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "promotion item has no shortening value")
		}
		updated := &papi.UpdatedData{}
		if _, err := viber.ConsumeExchangeItemStacks(ctx, suid, req.GetItemId(), int64(req.GetAmount()), updated); err != nil {
			if errors.Is(err, viber.ErrExchangeInsufficientResource) {
				return nil, status.Error(codes.ResourceExhausted, "promotion shortening item is insufficient")
			}
			return nil, err
		}
		shortened, err := activityShorteningMillis(item.GetValue(), req.GetAmount())
		if err != nil {
			return nil, err
		}
		if progress.GetStartTime() < math.MinInt64+shortened || progress.GetFinishTime() < math.MinInt64+shortened {
			return nil, status.Error(codes.FailedPrecondition, "promotion shortening overflows")
		}
		progress.StartTime -= shortened
		progress.FinishTime -= shortened
		if _, err := advancePromotion(ctx, suid, progress, time.Now().UnixMilli()); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserPromotionRun(ctx, suid, progress); err != nil {
			return nil, err
		}
		info, err := opuser.FindUserPromotionInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		if info.ActivePromotion == nil {
			return nil, status.Error(codes.FailedPrecondition, "active promotion summary is missing")
		}
		info.ActivePromotion.FinishTime = progress.GetFinishTime()
		if err := opuser.UpsertUserPromotionInfo(ctx, suid, info); err != nil {
			return nil, err
		}
		response = &papi.ActivityUsePromotionItemResponse{
			Progress:       progress,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *ActivityServer) ReceivePromotionReward(ctx context.Context, _ *papi.Empty) (*papi.ActivityReceivePromotionRewardResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityReceivePromotionRewardResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserPromotionRun(ctx, suid)
		if err != nil {
			return nil, activityRunError("promotion", err)
		}
		characters, err := advancePromotion(ctx, suid, progress, time.Now().UnixMilli())
		if err != nil {
			return nil, err
		}
		settled, err := settlePromotionRewards(ctx, suid, progress, characters, false)
		if err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserPromotionRun(ctx, suid, progress); err != nil {
			return nil, err
		}
		response = &papi.ActivityReceivePromotionRewardResponse{
			Rewards:               settled.rewards,
			BonusRewards:          []*ptransaction.Reward{},
			CompletedStep:         progress.GetCompletedStep(),
			Progress:              progress,
			ActivityLevelProgress: settled.levelProgress,
			CampaignEffects:       progress.GetCampaignEffects(),
			CampaignDropRewards:   []*ptransaction.Reward{},
			CommonResponse:        viber.NewCommonResponse(settled.updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *ActivityServer) FetchPromotionSteps(ctx context.Context, req *papi.ActivityFetchPromotionStepsRequest) (*papi.ActivityFetchPromotionStepsResponse, error) {
	if req == nil || req.GetStep() < 0 {
		return nil, viber.InvalidArgumentf("step must not be negative")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityFetchPromotionStepsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserPromotionRun(ctx, suid)
		if err != nil {
			return nil, activityRunError("promotion", err)
		}
		if _, err := advancePromotion(ctx, suid, progress, time.Now().UnixMilli()); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserPromotionRun(ctx, suid, progress); err != nil {
			return nil, err
		}
		response = &papi.ActivityFetchPromotionStepsResponse{
			Steps:          []*papi.PromotionStep{},
			CommonResponse: viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *ActivityServer) FinishPromotion(ctx context.Context, _ *papi.Empty) (*papi.ActivityFinishPromotionResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityFinishPromotionResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserPromotionRun(ctx, suid)
		if err != nil {
			return nil, activityRunError("promotion", err)
		}
		characters, err := advancePromotion(ctx, suid, progress, time.Now().UnixMilli())
		if err != nil {
			return nil, err
		}
		settled, err := settlePromotionRewards(ctx, suid, progress, characters, true)
		if err != nil {
			return nil, err
		}
		info, err := opuser.FindUserPromotionInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		info.ActivePromotion = nil
		if err := opuser.UpsertUserPromotionInfo(ctx, suid, info); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserPromotionRun(ctx, suid); err != nil {
			return nil, err
		}
		response = &papi.ActivityFinishPromotionResponse{
			Rewards:               settled.rewards,
			BonusRewards:          []*ptransaction.Reward{},
			ActivityLevelProgress: settled.levelProgress,
			TotalSteps:            progress.GetCompletedStep(),
			Characters:            progress.GetCharacters(),
			CampaignEffects:       progress.GetCampaignEffects(),
			CampaignDropRewards:   []*ptransaction.Reward{},
			CommonResponse:        viber.NewCommonResponse(settled.updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

type promotionSettlement struct {
	rewards       []*ptransaction.Reward
	levelProgress *papi.ActivityLevelProgress
	updated       *papi.UpdatedData
}

func settlePromotionRewards(
	ctx context.Context,
	suid string,
	progress *papi.PromotionProgress,
	characters []*ptransaction.UserCharacter,
	finish bool,
) (*promotionSettlement, error) {
	updated := &papi.UpdatedData{}
	runState, err := opuser.FindUserPromotionRunState(ctx, suid)
	if err != nil {
		return nil, err
	}
	if !runState.AccountingInitialized {
		// Compatibility for a run created before the accounting sidecar was
		// introduced. Pending gold is only used once to reconstruct the old
		// settlement cursor; every subsequent claim uses completed steps.
		legacyPendingSteps := int64(0)
		if progress.GetStepRewardGold() > 0 {
			legacyPendingSteps = int64(progress.GetRewardGoldAmount()) / int64(progress.GetStepRewardGold())
		}
		runState.LastSettledCompletedStep = max(int64(0), int64(progress.GetCompletedStep())-legacyPendingSteps)
		runState.FanRewardStepRemainder = runState.LastSettledCompletedStep % 20
		runState.AccountingInitialized = true
	}
	if runState.LastSettledCompletedStep < 0 || runState.LastSettledCompletedStep > int64(progress.GetCompletedStep()) {
		return nil, status.Error(codes.FailedPrecondition, "promotion settlement cursor is invalid")
	}
	pendingSteps := int64(progress.GetCompletedStep()) - runState.LastSettledCompletedStep
	info, err := opuser.FindUserPromotionInfo(ctx, suid)
	if err != nil {
		return nil, err
	}
	promotion := activityPromotionByID(info, progress.GetActivityPromotionId())
	if promotion == nil {
		return nil, status.Error(codes.FailedPrecondition, "active promotion definition is missing")
	}
	finishedAt := time.Now().UnixMilli()
	maxLevel, err := viber.MaxUnlockedPromotionLevel(ctx, suid, promotion.GetId(), finishedAt, promotion.GetLevel())
	if err != nil {
		return nil, err
	}
	before := &papi.ActivityLevelInfo{
		Exp:             promotion.GetExp(),
		Level:           promotion.GetLevel(),
		NextLevelLocked: promotionNextLevelLocked(promotion, promotion.GetExp()),
	}
	activityExpGain := pendingSteps * int64(progress.GetStepRewardActivityExp())
	promotion.Exp = promotionInt32(int64(promotion.GetExp()) + activityExpGain)
	after := &papi.ActivityLevelInfo{
		Exp:             promotion.GetExp(),
		Level:           promotion.GetLevel(),
		NextLevelLocked: promotionNextLevelLocked(promotion, promotion.GetExp()),
	}
	if err := opuser.UpsertUserPromotionInfo(ctx, suid, info); err != nil {
		return nil, err
	}
	fanRewardSteps := runState.FanRewardStepRemainder + pendingSteps
	fanGain := fanRewardSteps / 20
	runState.FanRewardStepRemainder = fanRewardSteps % 20
	fanGain, err = viber.ActivityCampaignBonusAmount(
		fanGain,
		penum.ActivityCampaignEffectType_ActivityCampaignEffectType_PromotionFanUp,
		progress.GetCampaignEffects(),
	)
	if err != nil {
		return nil, err
	}
	for _, character := range characters {
		characterExpGain := pendingSteps * int64(progress.GetStepRewardCharacterActivityExp())
		if characterExpGain > 0 && character.GetActivityExp() > math.MaxInt64-characterExpGain {
			return nil, status.Error(codes.FailedPrecondition, "character activity exp overflows")
		}
		character.ActivityExp += characterExpGain
		if err := addPromotionFans(character, promotion.GetMaxFanAmount(), fanGain); err != nil {
			return nil, err
		}
		if err := viber.RecalculateAudienceCandidateAmount(ctx, character); err != nil {
			return nil, err
		}
		if finish {
			character.InActivity = false
			character.StaminaUpdatedTime = finishedAt
		}
		if err := updateCharacterActivityLevel(ctx, character); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
			return nil, err
		}
	}
	updated.Characters = characters
	rewards := make([]*ptransaction.Reward, 0, 3)
	managerExp := pendingSteps * int64(progress.GetStepRewardManagerExp())
	if managerExp > 0 {
		reward := &ptransaction.Reward{ResourceType: penum.ResourceType_ResourceType_ManagerExp, Amount: managerExp}
		if err := viber.GrantActivityReward(ctx, suid, reward, updated); err != nil {
			return nil, err
		}
		rewards = append(rewards, reward)
	}
	gold := int64(progress.GetRewardGoldAmount())
	if gold > 0 {
		reward := &ptransaction.Reward{ResourceType: penum.ResourceType_ResourceType_Gold, Amount: gold}
		if err := viber.GrantActivityReward(ctx, suid, reward, updated); err != nil {
			return nil, err
		}
		rewards = append(rewards, reward)
	}
	cardEnhance := int64(progress.GetRewardCardEnhanceItemAmount())
	if cardEnhance > 0 {
		reward := &ptransaction.Reward{
			ResourceType: penum.ResourceType_ResourceType_Item,
			ResourceId:   "item-card_enhance",
			Amount:       cardEnhance,
		}
		if err := viber.GrantActivityReward(ctx, suid, reward, updated); err != nil {
			return nil, err
		}
		rewards = append(rewards, reward)
	}
	dropRewards, err := settlePromotionDropRewards(ctx, suid, progress.GetRewards(), updated)
	if err != nil {
		return nil, err
	}
	rewards = append(rewards, dropRewards...)
	if pendingSteps > 0 {
		settingID, err := config.GetGameSettingId()
		if err != nil {
			return nil, err
		}
		setting, err := opmaster.FindSetting(ctx, settingID)
		if err != nil {
			return nil, err
		}
		totalCount, err := opuser.FindUserTotalCount(ctx, suid)
		if err != nil {
			return nil, err
		}
		minutes := pendingSteps * int64(setting.GetSecondsPerActivityPromotionStep()) / 60
		if minutes > 0 && totalCount.GetActivityPromotionMinutes() > math.MaxInt64-minutes {
			return nil, status.Error(codes.FailedPrecondition, "activity promotion minutes overflows")
		}
		totalCount.ActivityPromotionMinutes += minutes
		if err := opuser.UpsertUserTotalCount(ctx, suid, totalCount); err != nil {
			return nil, err
		}
		updated.TotalCount = totalCount
	}
	progress.RewardGoldAmount = 0
	progress.RewardCardEnhanceItemAmount = 0
	progress.Rewards = []*papi.ActivityPromotionReward{}
	runState.LastSettledCompletedStep = int64(progress.GetCompletedStep())
	if err := opuser.UpsertUserPromotionRunState(ctx, runState); err != nil {
		return nil, err
	}
	return &promotionSettlement{
		rewards: rewards,
		levelProgress: &papi.ActivityLevelProgress{
			MaxActivityLevel: maxLevel,
			Before:           before,
			After:            after,
		},
		updated: updated,
	}, nil
}

func promotionNextLevelLocked(promotion *papi.Promotion, exp int32) bool {
	return promotion != nil && promotion.GetNextLevelRequiredExp() > 0 &&
		exp >= promotion.GetNextLevelRequiredExp() && !promotion.GetIsNextLevelUnlocked()
}

func advancePromotion(ctx context.Context, suid string, progress *papi.PromotionProgress, now int64) ([]*ptransaction.UserCharacter, error) {
	if progress == nil || len(progress.GetCharacters()) != 3 || progress.GetStepConsumptionStamina() <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "active promotion data is invalid")
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return nil, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return nil, err
	}
	stepMillis := int64(setting.GetSecondsPerActivityPromotionStep()) * 1000
	if stepMillis <= 0 || progress.GetFinishTime() < progress.GetStartTime() {
		return nil, status.Error(codes.FailedPrecondition, "promotion timing is invalid")
	}
	maxSteps := (progress.GetFinishTime() - progress.GetStartTime()) / stepMillis
	completed := int64(0)
	if now > progress.GetStartTime() {
		completed = (now - progress.GetStartTime()) / stepMillis
	}
	if completed > maxSteps {
		completed = maxSteps
	}
	if completed > math.MaxInt32 {
		completed = math.MaxInt32
	}
	if completed < int64(progress.GetCompletedStep()) {
		completed = int64(progress.GetCompletedStep())
	}
	characters := make([]*ptransaction.UserCharacter, 0, len(progress.GetCharacters()))
	for _, selected := range progress.GetCharacters() {
		character, err := opuser.FindUserCharacter(ctx, suid, selected.GetCharacterId())
		if err != nil {
			return nil, err
		}
		characters = append(characters, character)
	}
	delta := completed - int64(progress.GetCompletedStep())
	consumption := progress.GetStepConsumptionStamina()
	for step := int64(0); step < delta; step++ {
		start := int((int64(progress.GetCompletedStep()) + step) % int64(len(characters)))
		consumed := false
		for offset := 0; offset < len(characters); offset++ {
			character := characters[(start+offset)%len(characters)]
			if character.ActivityStamina >= consumption {
				character.ActivityStamina -= consumption
				consumed = true
				break
			}
		}
		if !consumed {
			completed = int64(progress.GetCompletedStep()) + step
			delta = step
			break
		}
	}
	if delta > 0 {
		for _, character := range characters {
			character.StaminaUpdatedTime = now
			if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
				return nil, err
			}
		}
		progress.CompletedStep = int32(completed)
		gold, err := promotionRewardDelta(
			completed-delta,
			completed,
			progress.GetStepRewardGold(),
			penum.ActivityCampaignEffectType_ActivityCampaignEffectType_PromotionGoldUp,
			progress.GetCampaignEffects(),
		)
		if err != nil {
			return nil, err
		}
		cardEnhance, err := promotionRewardDelta(
			completed-delta,
			completed,
			progress.GetStepRewardCardEnhanceItem(),
			penum.ActivityCampaignEffectType_ActivityCampaignEffectType_PromotionCardEnhanceItemUp,
			progress.GetCampaignEffects(),
		)
		if err != nil {
			return nil, err
		}
		progress.RewardGoldAmount = promotionInt32(int64(progress.GetRewardGoldAmount()) + gold)
		progress.RewardCardEnhanceItemAmount = promotionInt32(int64(progress.GetRewardCardEnhanceItemAmount()) + cardEnhance)
		if err := appendPromotionMultiStepRewards(ctx, progress, completed-delta, completed); err != nil {
			return nil, err
		}
	}
	return characters, nil
}

func promotionRewardDelta(
	previousStep int64,
	completedStep int64,
	perStep int32,
	effectType penum.ActivityCampaignEffectType,
	effects []*papi.ActivityCampaignEffect,
) (int64, error) {
	if previousStep < 0 || completedStep < previousStep || perStep < 0 {
		return 0, status.Error(codes.FailedPrecondition, "promotion reward interval is invalid")
	}
	if perStep > 0 && completedStep > math.MaxInt64/int64(perStep) {
		return 0, status.Error(codes.FailedPrecondition, "promotion reward amount overflows")
	}
	completed, err := viber.ActivityCampaignBonusAmount(completedStep*int64(perStep), effectType, effects)
	if err != nil {
		return 0, err
	}
	previous, err := viber.ActivityCampaignBonusAmount(previousStep*int64(perStep), effectType, effects)
	if err != nil {
		return 0, err
	}
	return completed - previous, nil
}

func addPromotionFans(character *ptransaction.UserCharacter, maxFanAmount int32, amount int64) error {
	if character == nil || maxFanAmount <= 0 || amount <= 0 {
		return nil
	}
	total := int64(character.GetLightFanAmount()) + int64(character.GetMiddleFanAmount()) + int64(character.GetHeavyFanAmount())
	if total < 0 {
		return status.Error(codes.FailedPrecondition, "character fan amount is invalid")
	}
	remaining := int64(maxFanAmount) - total
	if remaining <= 0 {
		return nil
	}
	amount = min(amount, remaining)
	if int64(character.GetLightFanAmount()) > math.MaxInt32-amount {
		return status.Error(codes.FailedPrecondition, "character fan amount overflows")
	}
	character.LightFanAmount += int32(amount)
	return nil
}

func appendPromotionMultiStepRewards(
	ctx context.Context,
	progress *papi.PromotionProgress,
	previousStep int64,
	completedStep int64,
) error {
	if progress.GetMultiStepRewardId() == "" || completedStep/20 <= previousStep/20 {
		return nil
	}
	reward, err := opmaster.FindReward(ctx, progress.GetMultiStepRewardId())
	if err != nil {
		return err
	}
	resources := reward.GetResources()
	if len(resources) == 0 {
		return status.Errorf(codes.FailedPrecondition, "promotion reward %q has no resources", progress.GetMultiStepRewardId())
	}
	for milestone := previousStep/20 + 1; milestone <= completedStep/20; milestone++ {
		index, err := viber.CryptoRandomIndex(len(resources))
		if err != nil {
			return err
		}
		resource := resources[index]
		if resource == nil || resource.GetAmount() < 0 {
			return status.Errorf(codes.FailedPrecondition, "promotion reward %q is invalid", progress.GetMultiStepRewardId())
		}
		amount := int64(resource.GetAmount())
		if amount == 0 {
			amount = 1
		}
		progress.Rewards = append(progress.Rewards, &papi.ActivityPromotionReward{
			ResourceType: resource.GetResourceType(),
			ResourceId:   resource.GetResourceId(),
			Amount:       amount,
		})
	}
	return nil
}

type promotionDropRewardKey struct {
	resourceType penum.ResourceType
	resourceID   string
}

func settlePromotionDropRewards(
	ctx context.Context,
	suid string,
	drops []*papi.ActivityPromotionReward,
	updated *papi.UpdatedData,
) ([]*ptransaction.Reward, error) {
	amounts := make(map[promotionDropRewardKey]int64, len(drops))
	order := make([]promotionDropRewardKey, 0, len(drops))
	for _, drop := range drops {
		if drop == nil || drop.GetAmount() <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "promotion drop reward is invalid")
		}
		key := promotionDropRewardKey{resourceType: drop.GetResourceType(), resourceID: drop.GetResourceId()}
		if _, exists := amounts[key]; !exists {
			order = append(order, key)
		}
		if amounts[key] > math.MaxInt64-drop.GetAmount() {
			return nil, status.Error(codes.FailedPrecondition, "promotion drop reward overflows")
		}
		amounts[key] += drop.GetAmount()
	}
	rewards := make([]*ptransaction.Reward, 0, len(order))
	for _, key := range order {
		reward := &ptransaction.Reward{
			ResourceType: key.resourceType,
			ResourceId:   key.resourceID,
			Amount:       amounts[key],
		}
		if err := viber.GrantActivityReward(ctx, suid, reward, updated); err != nil {
			return nil, err
		}
		rewards = append(rewards, reward)
	}
	return rewards, nil
}

func updateCharacterActivityLevel(ctx context.Context, character *ptransaction.UserCharacter) error {
	for {
		next, err := opmaster.FindCharacterActivityLevel(ctx, int64(character.GetActivityLevel()+1))
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil
		}
		if err != nil {
			return err
		}
		if character.GetActivityExp() < int64(next.GetRequiredExp()) {
			return nil
		}
		character.ActivityLevel = next.GetLevel()
	}
}

func activityPromotionByID(info *papi.PromotionInfo, id string) *papi.Promotion {
	for _, promotion := range info.GetPromotions() {
		if promotion.GetId() == id {
			return promotion
		}
	}
	return nil
}

func promotionStepExperience(gold int32) (manager, activity, character int32) {
	if gold <= 0 {
		return 1, 1, 1
	}
	manager = max(1, promotionInt32((int64(gold)+4)/5-3))
	activity = max(1, promotionInt32(int64(gold)/10+5))
	character = max(1, promotionInt32(int64(gold)/10+10))
	return manager, activity, character
}

func activityShorteningMillis(value, amount int32) (int64, error) {
	if value <= 0 || amount <= 0 {
		return 0, viber.InvalidArgumentf("shortening value and amount must be positive")
	}
	minutes := int64(value) * int64(amount)
	millisPerMinute := int64(time.Minute / time.Millisecond)
	if minutes > math.MaxInt64/millisPerMinute {
		return 0, status.Error(codes.InvalidArgument, "shortening duration overflows")
	}
	return minutes * millisPerMinute, nil
}

func promotionInt32(value int64) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	if value < math.MinInt32 {
		return math.MinInt32
	}
	return int32(value)
}

func activityRunError(name string, err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return viber.FailedPreconditionf("no %s is active", name)
	}
	return err
}
