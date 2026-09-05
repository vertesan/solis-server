package market

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *MarketServer) Reset(ctx context.Context, req *papi.MarketResetRequest) (*papi.MarketResetResponse, error) {
	suid := viber.GetSuid(ctx).String()
	now := time.Now()
	catalog, err := loadMarketCatalog()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load market catalog: %v", err)
	}

	var response *papi.MarketResetResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		state, err := findAndRefreshMarketState(ctx, suid, now, catalog)
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		if req.GetIsFree() {
			boundary := jSTHalfDayStart(now).UnixMilli()
			if state.FreeResetBoundaryTime == boundary {
				return nil, errMarketFreeResetUsed
			}
			state.FreeResetBoundaryTime = boundary
		} else {
			if state.PaidResetCount >= catalog.PaidResetLimit {
				return nil, errMarketPaidResetLimit
			}
			if err := viber.ConsumeExchangeResource(
				ctx,
				suid,
				penum.ResourceType_ResourceType_Stone,
				"",
				int64(catalog.RequiredResetStoneAmount),
				updated,
			); err != nil {
				return nil, err
			}
			state.PaidResetCount++
		}

		items, err := generateMarketItems(ctx, suid, catalog, now)
		if err != nil {
			return nil, err
		}
		state.MarketItems = items
		if err := opuser.UpsertUserMarketState(ctx, state); err != nil {
			return nil, err
		}
		response = &papi.MarketResetResponse{
			MarketItems:              cloneMarketItems(items),
			FreeResetRemainCount:     marketFreeResetRemainCount(state, now),
			ResetRemainCount:         marketPaidResetRemainCount(state, catalog),
			RequiredResetStoneAmount: catalog.RequiredResetStoneAmount,
			CommonResponse:           viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		switch {
		case errors.Is(err, errMarketFreeResetUsed):
			return nil, status.Error(codes.FailedPrecondition, "free market reset is unavailable")
		case errors.Is(err, errMarketPaidResetLimit):
			return nil, status.Error(codes.FailedPrecondition, "market reset limit reached")
		case errors.Is(err, viber.ErrExchangeInsufficientResource):
			return nil, status.Error(codes.ResourceExhausted, "insufficient stones")
		default:
			return nil, err
		}
	}
	return response, nil
}
