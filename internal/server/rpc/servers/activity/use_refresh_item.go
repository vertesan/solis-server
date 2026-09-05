package activity

import (
	"context"
	"errors"
	"math"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ActivityServer) UseRefreshItem(ctx context.Context, req *papi.ActivityUseRefreshItemRequest) (*papi.ActivityUseRefreshItemResponse, error) {
	if req == nil || req.GetItemId() == "" || req.GetAmount() <= 0 {
		return nil, viber.InvalidArgumentf("a positive refresh item amount is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityUseRefreshItemResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserRefreshRun(ctx, suid)
		if err != nil {
			return nil, activityRunError("refresh", err)
		}
		item, err := opmaster.FindItem(ctx, req.GetItemId())
		if err != nil {
			return nil, err
		}
		if item.GetType() != penum.ItemType_ItemType_ActivityRefreshShorten {
			return nil, viber.InvalidArgumentf("item %q cannot shorten refreshes", req.GetItemId())
		}
		if item.GetValue() <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "refresh item has no shortening value")
		}
		updated := &papi.UpdatedData{}
		if _, err := viber.ConsumeExchangeItemStacks(ctx, suid, req.GetItemId(), int64(req.GetAmount()), updated); err != nil {
			if errors.Is(err, viber.ErrExchangeInsufficientResource) {
				return nil, status.Error(codes.ResourceExhausted, "refresh shortening item is insufficient")
			}
			return nil, err
		}
		shortened, err := activityShorteningMillis(item.GetValue(), req.GetAmount())
		if err != nil {
			return nil, err
		}
		if progress.GetStartTime() < math.MinInt64+shortened || progress.GetFinishTime() < math.MinInt64+shortened {
			return nil, status.Error(codes.FailedPrecondition, "refresh shortening overflows")
		}
		progress.StartTime -= shortened
		progress.FinishTime -= shortened
		if err := opuser.UpsertUserRefreshRun(ctx, suid, progress); err != nil {
			return nil, err
		}
		info, err := opuser.FindUserRefreshInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		if info.ActiveRefresh == nil {
			return nil, status.Error(codes.FailedPrecondition, "active refresh summary is missing")
		}
		info.ActiveRefresh.FinishTime = progress.GetFinishTime()
		if err := opuser.UpsertUserRefreshInfo(ctx, suid, info); err != nil {
			return nil, err
		}
		response = &papi.ActivityUseRefreshItemResponse{
			Progress:       progress,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
