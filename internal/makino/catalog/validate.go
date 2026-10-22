package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/makino/reward"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"google.golang.org/protobuf/proto"
)

func validateShops(ctx context.Context, shops []*smodel.ShopCatalogEntry) error {
	ids := make(map[string]string)
	refs := referenceValidator{ctx: ctx, checked: make(map[string]bool)}
	for _, shop := range shops {
		if shop == nil || shop.Info == nil || shop.Info.GetId() != shop.ShopId {
			return fmt.Errorf("shopId must match info.id")
		}
		if len(shop.Items) == 0 {
			return fmt.Errorf("shop %q has no items; empty shops freeze the client", shop.ShopId)
		}
		if err := refs.check("condition", shop.Info.GetViewConditionId()); err != nil {
			return err
		}
		for _, item := range shop.Items {
			if item == nil || strings.TrimSpace(item.GetId()) == "" {
				return fmt.Errorf("shop %q has an item without an id", shop.ShopId)
			}
			if previous, duplicate := ids[item.GetId()]; duplicate {
				return fmt.Errorf("shop item %q appears in both %q and %q", item.GetId(), previous, shop.ShopId)
			}
			ids[item.GetId()] = shop.ShopId
			if err := refs.check("reward", item.GetRewardId()); err != nil {
				return err
			}
			if err := refs.check("consumption", item.GetConsumptionId()); err != nil {
				return err
			}
			if err := refs.check("condition", item.GetViewConditionId()); err != nil {
				return err
			}
			if err := refs.check("condition", item.GetUnlockConditionId()); err != nil {
				return err
			}
			if item.GetProductId() != "" {
				product, err := opmaster.FindStoreProduct(ctx, item.GetProductId())
				if err != nil {
					return fmt.Errorf("shop item %q product %q: %w", item.GetId(), item.GetProductId(), err)
				}
				if product.GetStoneAmount() > 0 {
					if shop.OriginalProductIds == nil {
						shop.OriginalProductIds = make(map[string]string)
					}
					if existing := shop.OriginalProductIds[item.GetId()]; existing != "" && existing != item.GetProductId() {
						return fmt.Errorf("shop item %q has conflicting product references", item.GetId())
					}
					shop.OriginalProductIds[item.GetId()] = item.GetProductId()
				}
			}
			normalizeShopItem(item)
		}
		for itemID, productID := range shop.OriginalProductIds {
			if ids[itemID] != shop.ShopId || productID == "" {
				return fmt.Errorf("invalid originalProductIds entry %q in shop %q", itemID, shop.ShopId)
			}
			product, err := opmaster.FindStoreProduct(ctx, productID)
			if err != nil {
				return fmt.Errorf("original product %q: %w", productID, err)
			}
			if product.GetStoneAmount() <= 0 {
				return fmt.Errorf("original product %q has no diamonds", productID)
			}
		}
		if pack := shop.LoginBonusPackageItem; pack != nil {
			if pack.ShopItem == nil {
				return fmt.Errorf("shop %q login package has no shopItem", shop.ShopId)
			}
			normalizeShopItem(pack.ShopItem)
			matched := false
			for _, item := range shop.Items {
				if proto.Equal(item, pack.ShopItem) {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("shop %q login package shopItem must also appear identically in items", shop.ShopId)
			}
			if pack.ShopItem.GetConsumptionId() != "" || pack.ShopItem.GetRewardId() != "" {
				return fmt.Errorf("shop %q login package must be a free entitlement", shop.ShopId)
			}
		}
	}
	return nil
}

func normalizeShopItem(item *papi.ShopItem) {
	item.ProductId = ""
	item.PurchaseLimit = 0
	item.LeftCount = 0
	item.ResetTimingType = penum.ResetTimingType_ResetTimingType_Never
	item.NextResetTime = 0
}

func validateBooths(ctx context.Context, booths []*papi.ExchangeBooth) error {
	ids := make(map[string]string)
	refs := referenceValidator{ctx: ctx, checked: make(map[string]bool)}
	for _, booth := range booths {
		if len(booth.GetExchanges()) == 0 {
			return fmt.Errorf("exchange booth %q has no items", booth.GetId())
		}
		if err := refs.check("condition", booth.GetViewConditionId()); err != nil {
			return err
		}
		if err := refs.check("condition", booth.GetUnlockConditionId()); err != nil {
			return err
		}
		for _, item := range booth.GetExchanges() {
			if strings.TrimSpace(item.GetId()) == "" {
				return fmt.Errorf("exchange booth %q has an item without an id", booth.GetId())
			}
			if previous, duplicate := ids[item.GetId()]; duplicate {
				return fmt.Errorf("exchange item %q appears in both %q and %q", item.GetId(), previous, booth.GetId())
			}
			ids[item.GetId()] = booth.GetId()
			if item.GetRequiredResourceAmount() < 0 || item.GetExchangeLimit() < 0 {
				return fmt.Errorf("exchange item %q has a negative cost or limit", item.GetId())
			}
			if item.GetRewardId() == "" {
				return fmt.Errorf("exchange item %q requires a rewardId", item.GetId())
			}
			if err := refs.check("reward", item.GetRewardId()); err != nil {
				return err
			}
			if err := refs.check("condition", item.GetViewConditionId()); err != nil {
				return err
			}
			if err := refs.check("condition", item.GetUnlockConditionId()); err != nil {
				return err
			}
		}
	}
	return nil
}

type referenceValidator struct {
	ctx     context.Context
	checked map[string]bool
}

func (v *referenceValidator) check(kind, id string) error {
	key := kind + ":" + id
	if id == "" || v.checked[key] {
		return nil
	}
	var err error
	switch kind {
	case "reward":
		resources, parseErr := reward.ParseAuto(id)
		err = parseErr
		if err == nil && resources == nil {
			_, err = opmaster.FindReward(v.ctx, id)
		}
	case "consumption":
		_, err = opmaster.FindConsumption(v.ctx, id)
	case "condition":
		_, err = opmaster.FindCondition(v.ctx, id)
	}
	if err != nil {
		return fmt.Errorf("%s %q: %w", kind, id, err)
	}
	v.checked[key] = true
	return nil
}
