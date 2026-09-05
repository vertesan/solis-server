package activity

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
		if err := reconcilePromotionInfo(ctx, suid, now, info); err != nil {
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
		characters, lineup, err := validateActivityLineup(
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

// reconcilePromotionInfo refreshes the condition-backed promotion gates before
// a direct Start call, which need not be preceded by Home.Enter.
func reconcilePromotionInfo(ctx context.Context, suid string, now int64, info *papi.PromotionInfo) error {
	return viber.ReconcilePromotionInfo(ctx, suid, now, viber.NewCommerceConditionEvaluator(ctx, suid, now), info)
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
