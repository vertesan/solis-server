package activity

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/salary"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

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
		if err := salary.RecordActivity(ctx, suid, progress.GetStartTime(), min(time.Now().UnixMilli(), progress.GetFinishTime()), int32(len(progress.GetCharacters()))); err != nil {
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
