package market

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *MarketServer) ListItem(ctx context.Context, _ *papi.Empty) (*papi.MarketListItemResponse, error) {
	suid := viber.GetSuid(ctx).String()
	now := time.Now()
	catalog, err := loadMarketCatalog()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load market catalog: %v", err)
	}

	var response *papi.MarketListItemResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		state, err := findAndRefreshMarketState(ctx, suid, now, catalog)
		if err != nil {
			return nil, err
		}
		response = &papi.MarketListItemResponse{
			MarketItems:              cloneMarketItems(state.MarketItems),
			NextResetTime:            nextJSTHalfDayStart(now).UnixMilli(),
			FreeResetRemainCount:     marketFreeResetRemainCount(state, now),
			ResetRemainCount:         marketPaidResetRemainCount(state, catalog),
			RequiredResetStoneAmount: catalog.RequiredResetStoneAmount,
			CommonResponse:           viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// nextJSTHalfDayStart returns the next 00:00 or 12:00 JST boundary.
func nextJSTHalfDayStart(now time.Time) time.Time {
	return jSTHalfDayStart(now).Add(12 * time.Hour)
}
