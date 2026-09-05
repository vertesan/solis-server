package servers

import (
	"context"
	"errors"
	"math"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ItemServer struct {
	papi.UnimplementedItemServer
}

func (s *ItemServer) Sell(ctx context.Context, req *papi.ItemSellRequest) (*papi.ItemSellResponse, error) {
	if req.GetItemId() == "" || req.GetAmount() <= 0 {
		return nil, viber.InvalidArgumentf("itemId and a positive amount are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ItemSellResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		item, err := opmaster.FindItem(ctx, req.ItemId)
		if err != nil {
			return nil, err
		}
		if item.SalePrice <= 0 {
			return nil, viber.FailedPreconditionf("item cannot be sold")
		}
		if req.Amount > math.MaxInt64/int64(item.SalePrice) {
			return nil, viber.InvalidArgumentf("sale proceeds overflow")
		}
		updated := &papi.UpdatedData{}
		if _, err := viber.ConsumeExchangeItemStacks(ctx, suid, req.ItemId, req.Amount, updated); err != nil {
			return nil, err
		}
		if err := viber.GrantGold(ctx, suid, req.Amount*int64(item.SalePrice), updated); err != nil {
			return nil, err
		}
		response = &papi.ItemSellResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if errors.Is(err, viber.ErrExchangeInsufficientResource) {
		return nil, status.Error(codes.ResourceExhausted, "insufficient items to sell")
	}
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
