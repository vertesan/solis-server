package activity

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ActivityServer) UsePromotionItem(ctx context.Context, req *papi.ActivityUsePromotionItemRequest) (*papi.ActivityUsePromotionItemResponse, error) {
	if req == nil || req.GetItemId() == "" || req.GetAmount() <= 0 {
		return nil, viber.InvalidArgumentf("a positive promotion item amount is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityUsePromotionItemResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserPromotionRun(ctx, suid)
		if err != nil {
			return nil, activityRunError("promotion", err)
		}
		item, err := opmaster.FindItem(ctx, req.GetItemId())
		if err != nil {
			return nil, err
		}
		switch item.GetType() {
		case penum.ItemType_ItemType_ActivityPromotionShorten,
			penum.ItemType_ItemType_ActivityPromotionSpecialShorten:
		default:
			return nil, viber.InvalidArgumentf("item %q cannot shorten promotions", req.GetItemId())
		}
		if item.GetValue() <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "promotion item has no shortening value")
		}
		updated := &papi.UpdatedData{}
		if _, err := viber.ConsumeExchangeItemStacks(ctx, suid, req.GetItemId(), int64(req.GetAmount()), updated); err != nil {
			if errors.Is(err, viber.ErrExchangeInsufficientResource) {
				return nil, status.Error(codes.ResourceExhausted, "promotion shortening item is insufficient")
			}
			return nil, err
		}
		shortened, err := activityShorteningMillis(item.GetValue(), req.GetAmount())
		if err != nil {
			return nil, err
		}
		if progress.GetStartTime() < math.MinInt64+shortened || progress.GetFinishTime() < math.MinInt64+shortened {
			return nil, status.Error(codes.FailedPrecondition, "promotion shortening overflows")
		}
		progress.StartTime -= shortened
		progress.FinishTime -= shortened
		if _, err := advancePromotion(ctx, suid, progress, time.Now().UnixMilli()); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserPromotionRun(ctx, suid, progress); err != nil {
			return nil, err
		}
		info, err := opuser.FindUserPromotionInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		if info.ActivePromotion == nil {
			return nil, status.Error(codes.FailedPrecondition, "active promotion summary is missing")
		}
		info.ActivePromotion.FinishTime = progress.GetFinishTime()
		if err := opuser.UpsertUserPromotionInfo(ctx, suid, info); err != nil {
			return nil, err
		}
		response = &papi.ActivityUsePromotionItemResponse{
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
