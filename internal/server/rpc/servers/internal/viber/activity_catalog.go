package viber

import (
	"context"
	"math"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func MaxUnlockedFanEventLevel(ctx context.Context, suid, fanEventID string, now int64, fallback int32) (int32, error) {
	masters, err := opmaster.FindAllActivityFanEvents(ctx)
	if err != nil {
		return 0, err
	}
	evaluator := NewCommerceConditionEvaluator(ctx, suid, now)
	maxLevel := fallback
	for _, master := range masters {
		if master.GetId() != fanEventID {
			continue
		}
		for _, level := range master.GetLevels() {
			unlocked, err := evaluator.Satisfied(level.GetUnlockConditionId())
			if err != nil {
				return 0, err
			}
			if unlocked {
				maxLevel = max(maxLevel, level.GetLevel())
			}
		}
		break
	}
	return maxLevel, nil
}

func MaxUnlockedPromotionLevel(ctx context.Context, suid, promotionID string, now int64, fallback int32) (int32, error) {
	masters, err := opmaster.FindAllActivityPromotions(ctx)
	if err != nil {
		return 0, err
	}
	evaluator := NewCommerceConditionEvaluator(ctx, suid, now)
	maxLevel := fallback
	for _, master := range masters {
		if master.GetId() != promotionID {
			continue
		}
		for _, level := range master.GetLevels() {
			unlocked, err := evaluator.Satisfied(level.GetUnlockConditionId())
			if err != nil {
				return 0, err
			}
			if unlocked {
				maxLevel = max(maxLevel, level.GetLevel())
			}
		}
		break
	}
	return maxLevel, nil
}

// ReconcileActivityInfos applies permanent condition-based unlocks and drops
// expired campaign effects before activity catalogs are exposed from Home.
// Unlocks are monotonic: once earned, a later condition change never relocks a
// user's activity.
func ReconcileActivityInfos(
	ctx context.Context,
	suid string,
	now int64,
	fanInfo *papi.FanEventInfo,
	promotionInfo *papi.PromotionInfo,
	refreshInfo *papi.RefreshInfo,
) error {
	evaluator := NewCommerceConditionEvaluator(ctx, suid, now)
	if err := reconcileFanEventInfo(ctx, suid, now, evaluator, fanInfo); err != nil {
		return err
	}
	if err := reconcilePromotionInfo(ctx, suid, now, evaluator, promotionInfo); err != nil {
		return err
	}
	return reconcileRefreshInfo(ctx, suid, now, evaluator, refreshInfo)
}

// ReconcileFanEventInfo refreshes the condition-backed fan-event gates before
// a direct Start call, which need not be preceded by Home.Enter.
func ReconcileFanEventInfo(ctx context.Context, suid string, now int64, info *papi.FanEventInfo) error {
	return reconcileFanEventInfo(ctx, suid, now, NewCommerceConditionEvaluator(ctx, suid, now), info)
}

// ReconcilePromotionInfo refreshes the condition-backed promotion gates before
// a direct Start call, which need not be preceded by Home.Enter.
func ReconcilePromotionInfo(ctx context.Context, suid string, now int64, info *papi.PromotionInfo) error {
	return reconcilePromotionInfo(ctx, suid, now, NewCommerceConditionEvaluator(ctx, suid, now), info)
}

// ReconcileRefreshInfo refreshes the condition-backed refresh gates before a
// direct Start call, which need not be preceded by Home.Enter.
func ReconcileRefreshInfo(ctx context.Context, suid string, now int64, info *papi.RefreshInfo) error {
	return reconcileRefreshInfo(ctx, suid, now, NewCommerceConditionEvaluator(ctx, suid, now), info)
}

// ActivityFanEventInfoForResponse clones the persisted catalog and hides its
// selectable entries while a fan event is active, matching the client-facing
// shape without discarding the user's stored progression.
func ActivityFanEventInfoForResponse(info *papi.FanEventInfo) *papi.FanEventInfo {
	if info == nil {
		return nil
	}
	projected := proto.Clone(info).(*papi.FanEventInfo)
	if projected.GetActiveFanEvent() != nil {
		projected.FanEvents = []*papi.FanEvent{}
	}
	return projected
}

// ActivityPromotionInfoForResponse is the promotion equivalent of
// ActivityFanEventInfoForResponse.
func ActivityPromotionInfoForResponse(info *papi.PromotionInfo) *papi.PromotionInfo {
	if info == nil {
		return nil
	}
	projected := proto.Clone(info).(*papi.PromotionInfo)
	if projected.GetActivePromotion() != nil {
		projected.Promotions = []*papi.Promotion{}
	}
	return projected
}

// ActivityRefreshInfoForResponse clones a persisted base-price catalog and
// projects current campaign discounts into the copy returned to the client.
// Persisting only base prices prevents repeated Home calls from compounding a
// discount and keeps StartRefresh charging from a stable source of truth.
func ActivityRefreshInfoForResponse(info *papi.RefreshInfo) (*papi.RefreshInfo, error) {
	if info == nil {
		return nil, nil
	}
	projected := proto.Clone(info).(*papi.RefreshInfo)
	if projected.GetActiveRefresh() != nil {
		projected.Refreshes = []*papi.Refresh{}
		return projected, nil
	}
	for _, refresh := range projected.GetRefreshes() {
		for _, level := range refresh.GetLevels() {
			cost, err := ActivityRefreshGoldCost(level.GetRequiredGold(), projected.GetCampaignEffects())
			if err != nil {
				return nil, err
			}
			if cost > math.MaxInt32 {
				return nil, status.Error(codes.FailedPrecondition, "discounted refresh price overflows")
			}
			level.RequiredGold = int32(cost)
		}
	}
	return projected, nil
}

func reconcileFanEventInfo(
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

func reconcilePromotionInfo(
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

func reconcileRefreshInfo(
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
