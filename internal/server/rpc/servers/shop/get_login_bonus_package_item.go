package shop

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ShopServer) GetLoginBonusPackageItem(ctx context.Context, req *papi.ShopGetLoginBonusPackageItemRequest) (*papi.ShopGetLoginBonusPackageItemResponse, error) {
	if req.GetShopId() == "" {
		return nil, status.Error(codes.InvalidArgument, "shopId is required")
	}
	suid := viber.GetSuid(ctx).String()
	evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
	entry, err := findAuthorizedShopEntry(ctx, suid, req.GetShopId(), evaluator)
	if err != nil {
		return nil, err
	}
	if entry.LoginBonusPackageItem == nil {
		return nil, status.Error(codes.NotFound, "login bonus package not found")
	}
	item := entry.LoginBonusPackageItem.GetShopItem()
	visible, err := evaluator.Satisfied(item.GetViewConditionId())
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, status.Error(codes.PermissionDenied, "shop item is locked")
	}
	unlocked, err := evaluator.Satisfied(item.GetUnlockConditionId())
	if err != nil {
		return nil, err
	}
	purchase, err := viber.FindUserShopPurchaseOrEmpty(ctx, suid)
	if err != nil {
		return nil, err
	}
	_, loginPackage, err := viber.RenderLoginBonusShop(entry, purchase)
	if err != nil {
		return nil, err
	}
	if loginPackage.ShopItem != nil {
		loginPackage.ShopItem.Unlocked = unlocked
	}
	return &papi.ShopGetLoginBonusPackageItemResponse{
		LoginBonusPackageItem: loginPackage,
		CommonResponse:        viber.NewCommonResponse(nil),
	}, nil
}
