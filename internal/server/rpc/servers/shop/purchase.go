package shop

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
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
)

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
		if isLoginBonusShopItem(entry, item.GetId()) {
			if item.GetConsumptionId() != "" || item.GetRewardId() != "" {
				return nil, status.Error(codes.FailedPrecondition, "login bonus package is not a free entitlement")
			}
			activateLoginBonusShop(purchase, entry)
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

func isLoginBonusShopItem(entry *smodel.ShopCatalogEntry, shopItemID string) bool {
	if entry == nil || entry.LoginBonusPackageItem == nil {
		return false
	}
	item := entry.LoginBonusPackageItem.GetShopItem()
	return item != nil && item.GetId() != "" && item.GetId() == shopItemID
}

// activateLoginBonusShop records the free package entitlement. It is
// intentionally idempotent because clients can retry Purchase after a lost
// response.
func activateLoginBonusShop(purchase *umodel.UserShopPurchase, entry *smodel.ShopCatalogEntry) bool {
	if purchase == nil || entry == nil || entry.LoginBonusPackageItem == nil {
		return false
	}
	itemID := entry.LoginBonusPackageItem.GetShopItem().GetId()
	if itemID == "" || viber.LoginBonusShopIsActive(purchase, entry) {
		return false
	}
	purchase.PurchasedItemIds = append(purchase.PurchasedItemIds, itemID)
	return true
}

func isShowcaseCommerceItemID(itemID string) bool {
	return strings.HasPrefix(itemID, viber.ShowcaseItemIDPrefix) || strings.HasPrefix(itemID, viber.ShowcaseMusicItemIDPrefix)
}

func findShowcaseShopItem(c *viber.ShowcaseCommerceCatalog, itemID string) (*smodel.ShopCatalogEntry, *papi.ShopItem, bool) {
	if c == nil {
		return nil, nil, false
	}
	entry, ok := c.ItemsByID[itemID]
	return entry.Shop, entry.Item, ok
}

var errShopPurchaseLimitExceeded = errors.New("shop purchase limit exceeded")

func findAuthorizedShopItemEntry(
	ctx context.Context,
	suid string,
	shopItemID string,
	evaluator *viber.CommerceConditionEvaluator,
) (*smodel.ShopCatalogEntry, *papi.ShopItem, error) {
	if isShowcaseCommerceItemID(shopItemID) {
		catalog, err := viber.BuildShowcaseCommerceCatalog(ctx, suid)
		if err != nil {
			return nil, nil, err
		}
		entry, item, ok := findShowcaseShopItem(catalog, shopItemID)
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

func findShopItem(entry *smodel.ShopCatalogEntry, shopItemID string) *papi.ShopItem {
	for _, item := range entry.Items {
		if item.GetId() == shopItemID {
			return item
		}
	}
	return nil
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
