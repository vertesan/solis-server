package activity

import (
	"context"
	"errors"
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
		isItemPromotion := !req.GetIsFree() && req.GetUseItemId() != ""
		if !isItemPromotion && progress.GetDailyPromoteCount() >= setting.GetActivityLessonPromoteLimitCount() {
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
		if !isItemPromotion {
			progress.DailyPromoteCount++
		}
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
