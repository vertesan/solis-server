package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ValidateGachaRewards is shared by administrative imports and the runtime
// renderer so a saved pool cannot contain a resource the server cannot grant.
func ValidateGachaRewards(ctx context.Context, rewards []*pmaster.RewardResource) error {
	for _, reward := range rewards {
		if reward.GetResourceId() == "" || reward.GetAmount() <= 0 {
			return status.Error(codes.FailedPrecondition, "gacha catalog reward is invalid")
		}
		var err error
		switch reward.GetResourceType() {
		case penum.ResourceType_ResourceType_Card:
			_, err = opmaster.FindCard(ctx, reward.GetResourceId())
		case penum.ResourceType_ResourceType_Item:
			_, err = opmaster.FindItem(ctx, reward.GetResourceId())
		case penum.ResourceType_ResourceType_Accessory:
			_, err = opmaster.FindAccessory(ctx, reward.GetResourceId())
		case penum.ResourceType_ResourceType_ShowcaseToy:
			_, err = opmaster.FindShowcaseToy(ctx, reward.GetResourceId())
		default:
			return status.Errorf(codes.Unimplemented, "gacha catalog reward type %s is unsupported", reward.GetResourceType())
		}
		if err != nil {
			return fmt.Errorf("invalid gacha catalog resource %q: %w", reward.GetResourceId(), err)
		}
	}
	return nil
}

func ValidateGachaRulesReferences(ctx context.Context, rules *smodel.GachaRules) error {
	if err := rules.Validate(); err != nil {
		return err
	}
	for _, item := range rules.PremiumItems {
		if err := ValidateGachaRewards(ctx, []*pmaster.RewardResource{{
			ResourceType: penum.ResourceType_ResourceType_Item, ResourceId: item.ItemID, Amount: item.Amount,
		}}); err != nil {
			return err
		}
	}
	for _, pools := range [][]smodel.GachaCatalogPool{rules.ItemGachas, rules.ShowcaseToyGachas} {
		for _, pool := range pools {
			if _, err := opmaster.FindGacha(ctx, pool.ID); err != nil {
				return fmt.Errorf("gacha pool %q: %w", pool.ID, err)
			}
			rewards := make([]*pmaster.RewardResource, 0, len(pool.Rewards))
			for _, reward := range pool.Rewards {
				rewards = append(rewards, reward.MasterReward())
			}
			if err := ValidateGachaRewards(ctx, rewards); err != nil {
				return fmt.Errorf("gacha pool %q: %w", pool.ID, err)
			}
		}
	}
	for _, pool := range rules.GuaranteedRarityGachas {
		if _, err := opmaster.FindGacha(ctx, pool.ID); err != nil {
			return fmt.Errorf("guaranteed-rarity gacha %q: %w", pool.ID, err)
		}
	}
	for _, continuous := range rules.ContinuousGachas {
		if _, err := opmaster.FindGacha(ctx, continuous.ID); err != nil {
			return fmt.Errorf("continuous gacha %q: %w", continuous.ID, err)
		}
		if _, err := opmaster.FindReward(ctx, continuous.RewardID); err != nil {
			return fmt.Errorf("continuous gacha reward %q: %w", continuous.RewardID, err)
		}
	}
	// Notice aliases deliberately permit missing notices: the renderer supplies
	// a dummy notice, and administrators manage that catalog independently.
	return nil
}

func validateExplicitGachaRoster(ctx context.Context, rotation *smodel.GachaCatalog, rules *smodel.GachaRules) error {
	refs := referenceValidator{ctx: ctx, checked: make(map[string]bool)}
	for _, id := range rotation.GachaIds {
		master, err := opmaster.FindGacha(ctx, id)
		if err != nil {
			return fmt.Errorf("gacha %q is not in master data: %w", id, err)
		}
		if len(master.GetButtonIds()) == 0 {
			return fmt.Errorf("gacha %q has no draw buttons", id)
		}
		for _, buttonID := range master.GetButtonIds() {
			button, err := opmaster.FindGachaButton(ctx, buttonID)
			if err != nil {
				return fmt.Errorf("gacha %q button %q: %w", id, buttonID, err)
			}
			if button.GetGachaId() != id || button.GetDrawCount() <= 0 {
				return fmt.Errorf("gacha %q button %q has invalid ownership or draw count", id, buttonID)
			}
			for _, consumptionID := range []string{button.GetConsumptionId(), button.GetDiscountConsumptionId()} {
				if err := refs.check("consumption", consumptionID); err != nil {
					return err
				}
			}
		}
		for _, exchangeID := range master.GetExchangeIds() {
			if _, err := opmaster.FindGachaExchange(ctx, exchangeID); err != nil {
				return fmt.Errorf("gacha %q exchange %q: %w", id, exchangeID, err)
			}
		}
		var pool []*pmaster.RewardResource
		switch master.GetGachaType() {
		case penum.GachaType_GachaType_Item:
			pool = rules.ItemPool(id)
		case penum.GachaType_GachaType_ShowcaseToy:
			pool = rules.ShowcaseToyPool(id)
		case penum.GachaType_GachaType_Premium:
			rewardID := "rd-gacha-ratio-" + strings.TrimPrefix(id, "gacha-") + "-01"
			reward, err := opmaster.FindReward(ctx, rewardID)
			if err != nil {
				return fmt.Errorf("gacha %q premium reward %q: %w", id, rewardID, err)
			}
			pool = reward.GetResources()
			if len(pool) != 1 {
				return fmt.Errorf("gacha %q premium reward requires exactly one resource", id)
			}
		default:
			continue
		}
		if len(pool) == 0 {
			return fmt.Errorf("gacha %q has no configured reward pool; configure gacha-rules first", id)
		}
		if err := ValidateGachaRewards(ctx, pool); err != nil {
			return fmt.Errorf("gacha %q: %w", id, err)
		}
	}
	return nil
}
