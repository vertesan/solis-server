package activity

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
)

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
