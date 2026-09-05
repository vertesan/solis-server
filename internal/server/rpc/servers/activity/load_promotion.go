package activity

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

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
