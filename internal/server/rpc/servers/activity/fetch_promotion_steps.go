package activity

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

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
