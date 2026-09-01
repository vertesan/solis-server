package servers

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type ShopServer struct {
	papi.UnimplementedShopServer
}

var errShopPurchaseLimitExceeded = errors.New("shop purchase limit exceeded")

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
	purchase, err := viber.FindUserShopPurchaseOrEmpty(ctx, suid)
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
		if viber.IsShowcaseCommerceShopID(entry.ShopId) || viber.IsShowcaseShopType(entry.Info.GetType()) {
			continue
		}
		if _, ok := allowed[entry.ShopId]; !ok {
			continue
		}
		visible, err := evaluator.Satisfied(entry.Info.GetViewConditionId())
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		hasContent, err := viber.ShopCatalogHasVisibleContent(evaluator, entry)
		if err != nil {
			return nil, err
		}
		if hasContent {
			info := proto.Clone(entry.Info).(*papi.ShopInfo)
			if entry.LoginBonusPackageItem != nil {
				info, _, err = viber.RenderLoginBonusShop(entry, purchase)
				if err != nil {
					return nil, err
				}
			}
			shops = append(shops, info)
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

func (s *ShopServer) Purchase(ctx context.Context, req *papi.ShopPurchaseRequest) (*papi.ShopPurchaseResponse, error) {
	if req.GetShopItemId() == "" {
		return nil, status.Error(codes.InvalidArgument, "shopItemId is required")
	}

	suid := viber.GetSuid(ctx).String()
	var response *papi.ShopPurchaseResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now()
		evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now.UnixMilli())
		entry, itemDefinition, err := findAuthorizedShopItemEntry(
			ctx, suid, req.GetShopItemId(), evaluator,
		)
		if err != nil {
			return nil, err
		}
		purchase, err := viber.FindUserShopPurchaseOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		item, visible, err := renderShopItem(evaluator, itemDefinition, purchase, now)
		if err != nil {
			return nil, err
		}
		if !visible || !item.GetUnlocked() {
			return nil, status.Error(codes.PermissionDenied, "shop item is locked")
		}
		if entry.Info.GetType() == penum.ShopType_ShopType_DefaultStoneShop {
			return nil, status.Error(codes.FailedPrecondition, "dia shop requires the unsupported purchase transaction API")
		}
		if item.GetProductId() != "" {
			return nil, status.Error(codes.FailedPrecondition, "real-money shop item requires the purchase transaction API")
		}
		if item.GetPurchaseLimit() > 0 && item.GetLeftCount() <= 0 {
			return nil, errShopPurchaseLimitExceeded
		}
		if viber.IsLoginBonusShopItem(entry, item.GetId()) {
			if item.GetConsumptionId() != "" || item.GetRewardId() != "" {
				return nil, status.Error(codes.FailedPrecondition, "login bonus package is not a free entitlement")
			}
			viber.ActivateLoginBonusShop(purchase, entry)
			if err := opuser.UpsertUserShopPurchase(ctx, purchase); err != nil {
				return nil, err
			}
			response = &papi.ShopPurchaseResponse{
				Rewards:        []*ptransaction.RewardResult{},
				CommonResponse: viber.NewCommonResponse(nil),
			}
			return nil, nil
		}

		rewards, err := viber.ResolveRewardResources(ctx, item.GetRewardId(), 1)
		if err != nil {
			return nil, err
		}
		if productId := entry.OriginalProductIds[item.GetId()]; productId != "" {
			product, err := opmaster.FindStoreProduct(ctx, productId)
			if err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					return nil, status.Errorf(codes.FailedPrecondition, "store product %q is missing from master data", productId)
				}
				return nil, err
			}
			if product.GetStoneAmount() <= 0 {
				return nil, status.Errorf(codes.FailedPrecondition, "store product %q has an invalid dia amount", productId)
			}
			rewards = append(rewards, &pmaster.RewardResource{
				ResourceType: penum.ResourceType_ResourceType_PaidStone,
				Amount:       product.GetStoneAmount(),
			})
		}
		if err := viber.ValidateExchangeRewardResources(rewards); err != nil {
			return nil, err
		}
		consumption, err := findShopConsumption(ctx, item.GetConsumptionId(), rewards)
		if err != nil {
			return nil, err
		}

		updated := &papi.UpdatedData{}
		if consumption != nil {
			for _, resource := range consumption.GetResources() {
				resourceID := ""
				if len(resource.GetResourceIds()) == 1 {
					resourceID = resource.GetResourceIds()[0]
				}
				if err := viber.ConsumeExchangeResource(
					ctx, suid, resource.GetResourceType(), resourceID, int64(resource.GetAmount()), updated,
				); err != nil {
					return nil, err
				}
			}
		}

		results := make([]*ptransaction.RewardResult, 0, len(rewards))
		for _, reward := range rewards {
			granted, err := viber.GrantExchangeRewards(ctx, suid, reward, updated)
			if err != nil {
				return nil, err
			}
			results = append(results, granted...)
		}
		results, err = viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}

		if item.GetPurchaseLimit() > 0 {
			current := purchase.PurchaseCounts[item.GetId()]
			if current < 0 || current == math.MaxInt32 {
				return nil, status.Error(codes.FailedPrecondition, "shop purchase count is invalid")
			}
			purchase.PurchaseCounts[item.GetId()] = current + 1
		}
		purchase.PurchasedItemIds = appendUniqueString(purchase.PurchasedItemIds, item.GetId())
		if err := opuser.UpsertUserShopPurchase(ctx, purchase); err != nil {
			return nil, err
		}

		response = &papi.ShopPurchaseResponse{
			Rewards:        results,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		switch {
		case mongo.IsDuplicateKeyError(err):
			return nil, status.Error(codes.FailedPrecondition, "shop purchase state changed concurrently")
		case errors.Is(err, viber.ErrExchangeInsufficientResource):
			return nil, status.Error(codes.ResourceExhausted, "insufficient shop resource")
		case errors.Is(err, errShopPurchaseLimitExceeded):
			return nil, status.Error(codes.FailedPrecondition, "shop purchase limit exceeded")
		default:
			return nil, err
		}
	}
	return response, nil
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

func findAuthorizedShopEntry(
	ctx context.Context,
	suid string,
	shopID string,
	evaluator *viber.CommerceConditionEvaluator,
) (*smodel.ShopCatalogEntry, error) {
	if viber.IsShowcaseCommerceShopID(shopID) {
		catalog, err := viber.BuildShowcaseCommerceCatalog(ctx, suid)
		if err != nil {
			return nil, err
		}
		entry, ok := catalog.FindShop(shopID)
		if !ok {
			return nil, status.Error(codes.NotFound, "shop has no available items")
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

func findAuthorizedShopItemEntry(
	ctx context.Context,
	suid string,
	shopItemID string,
	evaluator *viber.CommerceConditionEvaluator,
) (*smodel.ShopCatalogEntry, *papi.ShopItem, error) {
	if viber.IsShowcaseCommerceItemID(shopItemID) {
		catalog, err := viber.BuildShowcaseCommerceCatalog(ctx, suid)
		if err != nil {
			return nil, nil, err
		}
		entry, item, ok := catalog.FindItem(shopItemID)
		if !ok {
			return nil, nil, status.Error(codes.NotFound, "shop item not found")
		}
		visible, err := evaluator.Satisfied(entry.Info.GetViewConditionId())
		if err != nil {
			return nil, nil, err
		}
		if !visible {
			return nil, nil, status.Error(codes.PermissionDenied, "shop is locked")
		}
		return entry, item, nil
	}

	definition, err := opsolis.FindShopCatalogEntryByItemId(ctx, shopItemID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil, status.Error(codes.NotFound, "shop item not found")
		}
		return nil, nil, err
	}
	entry, err := findAuthorizedShopEntry(ctx, suid, definition.ShopId, evaluator)
	if err != nil {
		return nil, nil, err
	}
	item := findShopItem(entry, shopItemID)
	if item == nil {
		return nil, nil, status.Error(codes.NotFound, "shop item not found")
	}
	return entry, item, nil
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

func findShopItem(entry *smodel.ShopCatalogEntry, shopItemID string) *papi.ShopItem {
	for _, item := range entry.Items {
		if item.GetId() == shopItemID {
			return item
		}
	}
	return nil
}

func renderShopItem(
	evaluator *viber.CommerceConditionEvaluator,
	definition *papi.ShopItem,
	purchase *umodel.UserShopPurchase,
	now time.Time,
) (*papi.ShopItem, bool, error) {
	visible, err := evaluator.Satisfied(definition.GetViewConditionId())
	if err != nil || !visible {
		return nil, visible, err
	}
	unlocked, err := evaluator.Satisfied(definition.GetUnlockConditionId())
	if err != nil {
		return nil, false, err
	}
	item := proto.Clone(definition).(*papi.ShopItem)
	item.Unlocked = unlocked
	applyShopStock(item, purchase, now)
	return item, true, nil
}

func applyShopStock(item *papi.ShopItem, purchase *umodel.UserShopPurchase, now time.Time) {
	resetShopPurchaseCountIfNeeded(purchase, item, now)
	if item.GetPurchaseLimit() > 0 {
		item.LeftCount = max(item.GetPurchaseLimit()-purchase.PurchaseCounts[item.GetId()], 0)
	} else {
		item.LeftCount = 0
	}
	switch item.GetResetTimingType() {
	case penum.ResetTimingType_ResetTimingType_Daily,
		penum.ResetTimingType_ResetTimingType_Weekly,
		penum.ResetTimingType_ResetTimingType_Monthly:
		item.NextResetTime = purchase.ResetTimes[item.GetId()]
	default:
		item.NextResetTime = 0
	}
}

func resetShopPurchaseCountIfNeeded(purchase *umodel.UserShopPurchase, item *papi.ShopItem, now time.Time) {
	next := shopNextResetTime(now, item.GetResetTimingType())
	if next == 0 {
		delete(purchase.ResetTimes, item.GetId())
		return
	}
	if purchase.ResetTimes[item.GetId()] != next {
		purchase.PurchaseCounts[item.GetId()] = 0
		purchase.ResetTimes[item.GetId()] = next
	}
}

func shopNextResetTime(now time.Time, timing penum.ResetTimingType) int64 {
	switch timing {
	case penum.ResetTimingType_ResetTimingType_Daily:
		return viber.NextJSTDayStart(now).UnixMilli()
	case penum.ResetTimingType_ResetTimingType_Weekly:
		return viber.NextJSTWeekStart(now).UnixMilli()
	case penum.ResetTimingType_ResetTimingType_Monthly:
		return viber.NextJSTMonthStart(now).UnixMilli()
	default:
		return 0
	}
}

func findShopConsumption(
	ctx context.Context,
	consumptionID string,
	rewards []*pmaster.RewardResource,
) (*pmaster.Consumption, error) {
	if consumptionID == "" {
		return nil, nil
	}
	consumption, err := opmaster.FindConsumption(ctx, consumptionID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, status.Errorf(codes.FailedPrecondition, "consumption %q is missing from master data", consumptionID)
		}
		return nil, err
	}
	if len(consumption.GetResources()) == 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "consumption %q has no resources", consumptionID)
	}
	for _, resource := range consumption.GetResources() {
		if resource == nil || resource.GetAmount() <= 0 {
			return nil, status.Errorf(codes.FailedPrecondition, "consumption %q contains an invalid resource", consumptionID)
		}
		if len(resource.GetSubResourceIds()) != 0 || len(resource.GetResourceIds()) > 1 {
			return nil, status.Errorf(codes.Unimplemented, "consumption %q uses an unsupported resource selector", consumptionID)
		}
		if resource.GetResourceType() == penum.ResourceType_ResourceType_Item && len(resource.GetResourceIds()) != 1 {
			return nil, status.Errorf(codes.FailedPrecondition, "consumption %q does not identify an item", consumptionID)
		}
		if err := viber.ValidateExchangeResources(resource.GetResourceType(), rewards); err != nil {
			return nil, fmt.Errorf("invalid shop consumption %q: %w", consumptionID, err)
		}
	}
	return consumption, nil
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
