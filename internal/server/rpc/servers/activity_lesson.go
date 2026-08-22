package servers

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const lessonCardEnhanceItemID = "item-card_enhance"

type ActivityServer struct {
	papi.UnimplementedActivityServer
}

func (s *ActivityServer) ReceiveLessonReward(ctx context.Context, _ *papi.Empty) (*papi.ActivityReceiveLessonRewardResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityReceiveLessonRewardResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
		progress, err := viber.ReconcileActivityLessonProgress(ctx, suid, now)
		if err != nil {
			return nil, err
		}
		lessonInfo, err := viber.ReconcileActivityLessonInfo(ctx, suid, now)
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		rewards, err := receiveLessonRewards(ctx, suid, progress, 0, lessonInfo.GetCampaignEffects(), updated)
		if err != nil {
			return nil, err
		}
		if len(rewards) > 0 {
			settingID, err := config.GetGameSettingId()
			if err != nil {
				return nil, err
			}
			setting, err := opmaster.FindSetting(ctx, settingID)
			if err != nil {
				return nil, err
			}
			if err := advanceLessonReceiveAnchor(progress, now, setting.GetSecondsPerActivityLessonStep()); err != nil {
				return nil, err
			}
			progress.RewardGoldAmount = 0
			progress.RewardManagerExpAmount = 0
			progress.RewardCardEnhanceItemAmount = 0
			if err := opuser.UpsertUserLessonProgress(ctx, suid, progress); err != nil {
				return nil, err
			}
		}
		response = &papi.ActivityReceiveLessonRewardResponse{
			Rewards:             rewards,
			Progress:            progress,
			CampaignEffects:     lessonInfo.GetCampaignEffects(),
			CampaignDropRewards: []*ptransaction.Reward{},
			CommonResponse:      viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (s *ActivityServer) PromoteLesson(ctx context.Context, req *papi.ActivityPromoteLessonRequest) (*papi.ActivityPromoteLessonResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityPromoteLessonResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
		progress, err := viber.ReconcileActivityLessonProgress(ctx, suid, now)
		if err != nil {
			return nil, err
		}
		lessonInfo, err := viber.ReconcileActivityLessonInfo(ctx, suid, now)
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
		if progress.GetDailyPromoteCount() >= setting.GetActivityLessonPromoteLimitCount() {
			return nil, status.Error(codes.ResourceExhausted, "daily lesson promotion limit reached")
		}
		updated := &papi.UpdatedData{}
		if req.GetIsFree() {
			if req.GetUseItemId() != "" {
				return nil, status.Error(codes.InvalidArgument, "a free lesson promotion cannot use an item")
			}
			if progress.GetDailyPromoteCount() >= setting.GetActivityLessonPromoteFreeLimitCount() {
				return nil, status.Error(codes.ResourceExhausted, "daily free lesson promotion already used")
			}
		} else if req.GetUseItemId() != "" {
			item, err := opmaster.FindItem(ctx, req.GetUseItemId())
			if err != nil {
				return nil, err
			}
			if item.GetType() != penum.ItemType_ItemType_ActivityLessonPromote {
				return nil, status.Error(codes.InvalidArgument, "item cannot promote lessons")
			}
			if _, err := viber.ConsumeExchangeItemStacks(ctx, suid, item.GetId(), 1, updated); err != nil {
				if errors.Is(err, viber.ErrExchangeInsufficientResource) {
					return nil, status.Error(codes.ResourceExhausted, "lesson promotion item is insufficient")
				}
				return nil, err
			}
		} else {
			amount := int64(setting.GetActivityLessonPromoteRequiredStoneAmount())
			if amount <= 0 {
				return nil, status.Error(codes.FailedPrecondition, "lesson promotion stone cost is invalid")
			}
			if err := viber.ConsumeExchangeResource(ctx, suid, penum.ResourceType_ResourceType_Stone, "", amount, updated); err != nil {
				if errors.Is(err, viber.ErrExchangeInsufficientResource) {
					return nil, status.Error(codes.ResourceExhausted, "stones are insufficient")
				}
				return nil, err
			}
		}
		stepSeconds := setting.GetSecondsPerActivityLessonStep()
		if stepSeconds <= 0 || setting.GetActivityLessonPromoteEffectHours() <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "lesson promotion master settings are invalid")
		}
		bonusSteps := int64(setting.GetActivityLessonPromoteEffectHours()) * 60 * 60 / int64(stepSeconds)
		rewards, err := receiveLessonRewards(
			ctx, suid, progress, bonusSteps, lessonInfo.GetCampaignEffects(), updated,
		)
		if err != nil {
			return nil, err
		}
		progress.DailyPromoteCount++
		if err := advanceLessonReceiveAnchor(progress, now, stepSeconds); err != nil {
			return nil, err
		}
		progress.RewardGoldAmount = 0
		progress.RewardManagerExpAmount = 0
		progress.RewardCardEnhanceItemAmount = 0
		if err := opuser.UpsertUserLessonProgress(ctx, suid, progress); err != nil {
			return nil, err
		}
		response = &papi.ActivityPromoteLessonResponse{
			Rewards:             rewards,
			Progress:            progress,
			CampaignEffects:     lessonInfo.GetCampaignEffects(),
			CampaignDropRewards: []*ptransaction.Reward{},
			CommonResponse:      viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// advanceLessonReceiveAnchor advances across every complete elapsed step while
// preserving the sub-step remainder. Reward reconciliation separately caps
// the paid interval at twelve hours; advancing against now discards excess
// idle time instead of letting repeated receives bypass that cap. A promotion
// with no pending idle steps leaves both timestamps unchanged and only grants
// its explicit bonus window.
func advanceLessonReceiveAnchor(progress *papi.ActivityLessonProgress, now int64, stepSeconds int32) error {
	if progress == nil || stepSeconds <= 0 {
		return status.Error(codes.FailedPrecondition, "lesson step duration is invalid")
	}
	stepMillis := int64(stepSeconds) * 1000
	if now <= progress.GetLastReceiveTime() {
		return nil
	}
	steps := (now - progress.GetLastReceiveTime()) / stepMillis
	if steps <= 0 {
		return nil
	}
	if steps > (math.MaxInt64-progress.GetLastReceiveTime())/stepMillis {
		return status.Error(codes.FailedPrecondition, "lesson receive time overflows")
	}
	progress.LastReceiveTime += steps * stepMillis
	window := int64(12 * time.Hour / time.Millisecond)
	if progress.GetLastReceiveTime() > math.MaxInt64-window {
		return status.Error(codes.FailedPrecondition, "lesson reward window overflows")
	}
	progress.MaxRewardTime = progress.GetLastReceiveTime() + window
	return nil
}

func receiveLessonRewards(
	ctx context.Context,
	suid string,
	progress *papi.ActivityLessonProgress,
	bonusSteps int64,
	effects []*papi.ActivityCampaignEffect,
	updated *papi.UpdatedData,
) ([]*ptransaction.Reward, error) {
	amounts := []struct {
		resourceType penum.ResourceType
		resourceID   string
		pending      int64
		perStep      int32
		effectType   penum.ActivityCampaignEffectType
	}{
		{
			penum.ResourceType_ResourceType_Gold,
			"",
			int64(progress.GetRewardGoldAmount()),
			progress.GetStepRewardGold(),
			penum.ActivityCampaignEffectType_ActivityCampaignEffectType_LessonGoldUp,
		},
		{
			penum.ResourceType_ResourceType_ManagerExp,
			"",
			int64(progress.GetRewardManagerExpAmount()),
			progress.GetStepRewardManagerExp(),
			penum.ActivityCampaignEffectType_ActivityCampaignEffectType_LessonExpUp,
		},
		{
			penum.ResourceType_ResourceType_Item,
			lessonCardEnhanceItemID,
			int64(progress.GetRewardCardEnhanceItemAmount()),
			progress.GetStepRewardCardEnhanceItem(),
			penum.ActivityCampaignEffectType_ActivityCampaignEffectType_LessonCardEnhanceItemUp,
		},
	}
	rewards := make([]*ptransaction.Reward, 0, len(amounts))
	for _, value := range amounts {
		if bonusSteps > 0 && value.perStep > 0 {
			if bonusSteps > math.MaxInt64/int64(value.perStep) {
				return nil, status.Error(codes.FailedPrecondition, "lesson reward amount overflows")
			}
			bonus := bonusSteps * int64(value.perStep)
			if value.pending > math.MaxInt64-bonus {
				return nil, status.Error(codes.FailedPrecondition, "lesson reward amount overflows")
			}
			value.pending += bonus
		}
		adjusted, err := viber.ActivityCampaignBonusAmount(value.pending, value.effectType, effects)
		if err != nil {
			return nil, err
		}
		value.pending = adjusted
		if value.pending <= 0 {
			continue
		}
		reward := &ptransaction.Reward{
			ResourceType: value.resourceType,
			ResourceId:   value.resourceID,
			Amount:       value.pending,
		}
		if err := viber.GrantActivityReward(ctx, suid, reward, updated); err != nil {
			return nil, err
		}
		rewards = append(rewards, reward)
	}
	return rewards, nil
}
