package viber

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
)

func ReconcileFanEventInfo(
	ctx context.Context,
	suid string,
	now int64,
	evaluator *CommerceConditionEvaluator,
	info *papi.FanEventInfo,
) error {
	changed := pruneActivityCampaignEffects(&info.CampaignEffects, now)
	for _, fanEvent := range info.GetFanEvents() {
		if fanEvent == nil {
			continue
		}
		unlocked, err := activityConditionUnlock(
			evaluator, fanEvent.GetIsUnlocked(), fanEvent.GetUnlockConditionId(), true,
		)
		if err != nil {
			return err
		}
		if unlocked != fanEvent.GetIsUnlocked() {
			fanEvent.IsUnlocked = unlocked
			changed = true
		}
		hasNext := fanEvent.GetNextLevelRequiredExp() > 0 || fanEvent.GetNextLevelUnlockConditionId() != ""
		nextUnlocked, err := activityConditionUnlock(
			evaluator, fanEvent.GetIsNextLevelUnlocked(), fanEvent.GetNextLevelUnlockConditionId(), hasNext,
		)
		if err != nil {
			return err
		}
		if nextUnlocked != fanEvent.GetIsNextLevelUnlocked() {
			fanEvent.IsNextLevelUnlocked = nextUnlocked
			changed = true
		}
	}
	if changed {
		if err := opuser.UpsertUserFanEventInfo(ctx, suid, info); err != nil {
			return err
		}
	}
	return nil
}

func ReconcilePromotionInfo(
	ctx context.Context,
	suid string,
	now int64,
	evaluator *CommerceConditionEvaluator,
	info *papi.PromotionInfo,
) error {
	changed := pruneActivityCampaignEffects(&info.CampaignEffects, now)
	for _, promotion := range info.GetPromotions() {
		if promotion == nil {
			continue
		}
		unlocked, err := activityConditionUnlock(
			evaluator, promotion.GetIsUnlocked(), promotion.GetUnlockConditionId(), true,
		)
		if err != nil {
			return err
		}
		if unlocked != promotion.GetIsUnlocked() {
			promotion.IsUnlocked = unlocked
			changed = true
		}
		hasNext := promotion.GetNextLevelRequiredExp() > 0 || promotion.GetNextLevelUnlockConditionId() != ""
		nextUnlocked, err := activityConditionUnlock(
			evaluator, promotion.GetIsNextLevelUnlocked(), promotion.GetNextLevelUnlockConditionId(), hasNext,
		)
		if err != nil {
			return err
		}
		if nextUnlocked != promotion.GetIsNextLevelUnlocked() {
			promotion.IsNextLevelUnlocked = nextUnlocked
			changed = true
		}
	}
	if changed {
		if err := opuser.UpsertUserPromotionInfo(ctx, suid, info); err != nil {
			return err
		}
	}
	return nil
}

func ReconcileRefreshInfo(
	ctx context.Context,
	suid string,
	now int64,
	evaluator *CommerceConditionEvaluator,
	info *papi.RefreshInfo,
) error {
	changed := pruneActivityCampaignEffects(&info.CampaignEffects, now)
	for _, refresh := range info.GetRefreshes() {
		if refresh == nil {
			continue
		}
		unlocked, err := activityConditionUnlock(
			evaluator, refresh.GetIsUnlocked(), refresh.GetUnlockConditionId(), true,
		)
		if err != nil {
			return err
		}
		if unlocked != refresh.GetIsUnlocked() {
			refresh.IsUnlocked = unlocked
			changed = true
		}
		for _, level := range refresh.GetLevels() {
			if level == nil {
				continue
			}
			levelUnlocked, err := activityConditionUnlock(
				evaluator, level.GetIsUnlocked(), level.GetUnlockConditionId(), refresh.GetIsUnlocked(),
			)
			if err != nil {
				return err
			}
			if levelUnlocked != level.GetIsUnlocked() {
				level.IsUnlocked = levelUnlocked
				changed = true
			}
		}
	}
	if changed {
		if err := opuser.UpsertUserRefreshInfo(ctx, suid, info); err != nil {
			return err
		}
	}
	return nil
}

func activityConditionUnlock(
	evaluator *CommerceConditionEvaluator,
	alreadyUnlocked bool,
	conditionID string,
	hasTarget bool,
) (bool, error) {
	if alreadyUnlocked || !hasTarget {
		return alreadyUnlocked, nil
	}
	return evaluator.Satisfied(conditionID)
}

func pruneActivityCampaignEffects(effects *[]*papi.ActivityCampaignEffect, now int64) bool {
	if effects == nil || len(*effects) == 0 {
		return false
	}
	live := (*effects)[:0]
	for _, effect := range *effects {
		if effect != nil && effect.GetEndTime() > now {
			live = append(live, effect)
		}
	}
	if len(live) == len(*effects) {
		return false
	}
	*effects = live
	return true
}
