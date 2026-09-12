package card

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

// refundCardTrainingItems retains int64 amounts: high-level card EXP refunds
// exceed the int32 amount field used by master-data reward resources.
func refundCardTrainingItems(ctx context.Context, suid string, rewards []*ptransaction.Reward, updated *papi.UpdatedData) error {
	for _, reward := range rewards {
		if reward.GetResourceType() != penum.ResourceType_ResourceType_Item || reward.GetAmount() <= 0 {
			return fmt.Errorf("invalid card training refund")
		}
		if _, err := viber.GrantExchangeItem(ctx, suid, reward.ResourceId, reward.Amount, updated); err != nil {
			return err
		}
	}
	return nil
}

// cardAbilityTrainingRefunds returns material costs of the training being removed.
// A reset refunds base training above level one; removing support refunds only
// temporary training above the saved base levels.
func cardAbilityTrainingRefunds(ctx context.Context, master *pmaster.Card, card *ptransaction.UserCard, baseOnly bool) ([]*ptransaction.Reward, error) {
	currentSkills := []int32{card.SkillLevel1, card.SkillLevel2, card.SkillLevel3, card.SkillLevel4}
	baseSkills := []int32{card.BaseSkillLevel1, card.BaseSkillLevel2, card.BaseSkillLevel3, card.BaseSkillLevel4}
	currentAbilities := []int32{card.LiveAbilityLevel, card.ActivityAbilityLevel}
	baseAbilities := []int32{card.BaseLiveAbilityLevel, card.BaseActivityAbilityLevel}
	if baseOnly {
		currentSkills, baseSkills = baseSkills, []int32{1, 1, 1, 1}
		currentAbilities, baseAbilities = baseAbilities, []int32{1, 1}
	}
	var skillCost, abilityCost int64
	for i, id := range []string{master.SkillId1, master.SkillId2, master.SkillId3, master.SkillId4} {
		if currentSkills[i] <= max(baseSkills[i], 1) {
			continue
		}
		skill, err := opmaster.FindSkill(ctx, id)
		if err != nil {
			return nil, err
		}
		cost, err := cardTrainingTierCost(skill.Levels, baseSkills[i], currentSkills[i])
		if err != nil {
			return nil, err
		}
		skillCost += cost
	}
	if currentAbilities[0] > max(baseAbilities[0], 1) {
		ability, err := opmaster.FindLiveAbility(ctx, master.LiveAbilityId)
		if err != nil {
			return nil, err
		}
		cost, err := cardTrainingTierCost(ability.Levels, baseAbilities[0], currentAbilities[0])
		if err != nil {
			return nil, err
		}
		abilityCost += cost
	}
	if currentAbilities[1] > max(baseAbilities[1], 1) {
		ability, err := opmaster.FindActivityAbility(ctx, master.ActivityAbilityId)
		if err != nil {
			return nil, err
		}
		cost, err := cardTrainingTierCost(ability.Levels, baseAbilities[1], currentAbilities[1])
		if err != nil {
			return nil, err
		}
		abilityCost += cost
	}
	var rewards []*ptransaction.Reward
	for _, cost := range []struct {
		itemID string
		amount int64
	}{{"item-card_skill", skillCost}, {"item-card_ability", abilityCost}} {
		if cost.amount > 0 {
			rewards = append(rewards, &ptransaction.Reward{
				ResourceType: penum.ResourceType_ResourceType_Item,
				ResourceId:   cost.itemID, Amount: cost.amount,
			})
		}
	}
	return rewards, nil
}

func cardTrainingTierCost[T interface {
	GetLevel() int32
	GetRequiredItemAmount() int32
}](levels []T, base, current int32) (int64, error) {
	base = max(base, 1)
	var cost int64
	var count int32
	for _, level := range levels {
		if level.GetLevel() <= base || level.GetLevel() > current {
			continue
		}
		if level.GetRequiredItemAmount() < 0 {
			return 0, fmt.Errorf("card training has a negative material cost")
		}
		cost += int64(level.GetRequiredItemAmount())
		count++
	}
	if count != current-base {
		return 0, fmt.Errorf("card training level is missing from master data")
	}
	return cost, nil
}

const (
	cardEnhanceItemID    = "item-card_enhance"
	cardLimitBreakItemID = "item-card_limit_break"
	cardSkillItemID      = "item-card_skill"
	cardAbilityItemID    = "item-card_ability"
)
