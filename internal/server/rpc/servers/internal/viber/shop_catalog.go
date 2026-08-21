package viber

import "github.com/vertesan/solis-server/internal/db/model/solis/smodel"

// ShopCatalogHasVisibleContent keeps discovery responses consistent with the
// shop-specific endpoints. Locked items still count because ListItem returns
// them with Unlocked=false; only view conditions remove an item from the list.
func ShopCatalogHasVisibleContent(
	evaluator *CommerceConditionEvaluator,
	entry *smodel.ShopCatalogEntry,
) (bool, error) {
	if entry == nil || entry.Info == nil {
		return false, nil
	}
	for _, item := range entry.Items {
		if item == nil {
			continue
		}
		visible, err := evaluator.Satisfied(item.GetViewConditionId())
		if err != nil {
			return false, err
		}
		if visible {
			return true, nil
		}
	}
	if entry.LoginBonusPackageItem == nil {
		return false, nil
	}
	packageItem := entry.LoginBonusPackageItem.GetShopItem()
	if packageItem == nil {
		return false, nil
	}
	return evaluator.Satisfied(packageItem.GetViewConditionId())
}
