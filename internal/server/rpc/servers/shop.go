package servers

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type ShopServer struct {
	papi.UnimplementedShopServer
}

func (s *ShopServer) List(ctx context.Context, _ *papi.Empty) (*papi.ShopListResponse, error) {
	suid := viber.GetSuid(ctx).String()
	allowed, err := findUserShopAccess(ctx, suid)
	if err != nil {
		return nil, err
	}
	entries, err := opsolis.FindAllShopCatalogEntries(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Info != nil {
			allowed[entry.ShopId] = struct{}{}
		}
	}
	evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
	shops := make([]*papi.ShopInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.Info == nil {
			continue
		}
		if _, ok := allowed[entry.ShopId]; !ok {
			continue
		}
		visible, err := evaluator.Satisfied(entry.Info.GetViewConditionId())
		if err != nil {
			return nil, err
		}
		if visible {
			shops = append(shops, proto.Clone(entry.Info).(*papi.ShopInfo))
		}
	}
	return &papi.ShopListResponse{
		Shops:          shops,
		DokanInfos:     []*papi.DokanInfo{},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *ShopServer) ListItem(ctx context.Context, req *papi.ShopListItemRequest) (*papi.ShopListItemResponse, error) {
	if req.GetShopId() == "" {
		return nil, status.Error(codes.InvalidArgument, "shopId is required")
	}
	suid := viber.GetSuid(ctx).String()
	evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
	entry, err := findAuthorizedShopEntry(ctx, suid, req.GetShopId(), evaluator)
	if err != nil {
		return nil, err
	}
	items := make([]*papi.ShopItem, 0, len(entry.Items))
	for _, item := range entry.Items {
		visible, err := evaluator.Satisfied(item.GetViewConditionId())
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		unlocked, err := evaluator.Satisfied(item.GetUnlockConditionId())
		if err != nil {
			return nil, err
		}
		cloned := proto.Clone(item).(*papi.ShopItem)
		cloned.Unlocked = unlocked
		items = append(items, cloned)
	}
	return &papi.ShopListItemResponse{ShopItems: items, CommonResponse: viber.NewCommonResponse(nil)}, nil
}

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
	loginPackage := proto.Clone(entry.LoginBonusPackageItem).(*papi.LoginBonusPackageItem)
	if loginPackage.ShopItem != nil {
		loginPackage.ShopItem.Unlocked = unlocked
	}
	return &papi.ShopGetLoginBonusPackageItemResponse{
		LoginBonusPackageItem: loginPackage,
		CommonResponse:        viber.NewCommonResponse(nil),
	}, nil
}

func findAuthorizedShopEntry(
	ctx context.Context,
	suid string,
	shopID string,
	evaluator *viber.CommerceConditionEvaluator,
) (*smodel.ShopCatalogEntry, error) {
	_, err := findUserShopAccess(ctx, suid)
	if err != nil {
		return nil, err
	}
	entry, err := opsolis.FindShopCatalogEntry(ctx, shopID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, status.Error(codes.NotFound, "shop not found")
		}
		return nil, err
	}
	if entry.Info == nil {
		return nil, status.Error(codes.NotFound, "shop not found")
	}
	visible, err := evaluator.Satisfied(entry.Info.GetViewConditionId())
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, status.Error(codes.PermissionDenied, "shop is locked")
	}
	return entry, nil
}

func findUserShopAccess(ctx context.Context, suid string) (map[string]struct{}, error) {
	allowed := make(map[string]struct{})
	userShopIDs, err := opuser.FindUserShopIds(ctx, suid)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	if err == nil {
		for _, shopID := range userShopIDs.ShopIds {
			allowed[shopID] = struct{}{}
		}
	}
	showcaseShopIDs, err := opuser.FindUserShowcaseToyShopIds(ctx, suid)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	if err == nil {
		for _, shopID := range showcaseShopIDs.ShopIds {
			allowed[shopID] = struct{}{}
		}
	}
	return allowed, nil
}
