package activity

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

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
