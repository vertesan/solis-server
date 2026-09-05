package shop

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ShopServer) ListItem(ctx context.Context, req *papi.ShopListItemRequest) (*papi.ShopListItemResponse, error) {
	if req.GetShopId() == "" {
		return nil, status.Error(codes.InvalidArgument, "shopId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ShopListItemResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now()
		evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now.UnixMilli())
		entry, err := findAuthorizedShopEntry(ctx, suid, req.GetShopId(), evaluator)
		if err != nil {
			return nil, err
		}
		purchase, err := viber.FindUserShopPurchaseOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		items := make([]*papi.ShopItem, 0, len(entry.Items))
		for _, definition := range entry.Items {
			item, visible, err := renderShopItem(evaluator, definition, purchase, now)
			if err != nil {
				return nil, err
			}
			if visible {
				items = append(items, item)
			}
		}
		if len(items) == 0 {
			return nil, status.Error(codes.NotFound, "shop has no available items")
		}
		if err := opuser.UpsertUserShopPurchase(ctx, purchase); err != nil {
			return nil, err
		}
		response = &papi.ShopListItemResponse{
			ShopItems:      items,
			CommonResponse: viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}
