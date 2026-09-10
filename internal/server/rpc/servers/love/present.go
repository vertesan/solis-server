package love

import (
	"context"
	"errors"
	"math"

	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *LoveServer) Present(ctx context.Context, req *papi.LovePresentRequest) (*papi.LovePresentResponse, error) {
	var response *papi.LovePresentResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		suid := viber.GetSuid(ctx).String()
		catalog, state, love, err := loadLove(ctx, suid, req.GetLoveId())
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		if err := viber.ReconcileLoveMissions(ctx, suid, catalog.LoveID, state.Exp, updated); err != nil {
			return nil, err
		}
		if _, err := viber.ConsumeExchangeItemStacks(ctx, suid, catalog.Config.PresentItemID, 1, updated); err != nil {
			if errors.Is(err, viber.ErrExchangeInsufficientResource) {
				return nil, status.Error(codes.ResourceExhausted, "love present is insufficient")
			}
			return nil, err
		}
		if love.GetHiddenFavorability() > math.MaxInt64-catalog.Config.PresentFavorability {
			return nil, status.Error(codes.OutOfRange, "love favorability overflow")
		}
		love.HiddenFavorability += catalog.Config.PresentFavorability
		missions, err := viber.AdvanceUserMissions(ctx, suid, penum.MissionType_MissionType_ItemUsedAmount, 1, catalog.Config.PresentItemID)
		if err != nil {
			return nil, err
		}
		mergeLoveMissions(updated, missions)
		if err := storeLove(ctx, state, love, updated); err != nil {
			return nil, err
		}
		response = &papi.LovePresentResponse{HiddenFavorability: max(int64(0), love.GetHiddenFavorability()), CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, err
}
