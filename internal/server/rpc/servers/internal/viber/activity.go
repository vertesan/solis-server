package viber

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LiveActivityCampaignEffects removes expired and malformed campaign effects
// before an activity snapshots them into a run.
func LiveActivityCampaignEffects(effects []*papi.ActivityCampaignEffect, now int64) []*papi.ActivityCampaignEffect {
	live := make([]*papi.ActivityCampaignEffect, 0, len(effects))
	for _, effect := range effects {
		if effect != nil && effect.GetEndTime() > now {
			live = append(live, effect)
		}
	}
	return live
}

// ActivityRefreshGoldCost applies live refresh discount coefficients to a
// catalog base price. Coefficients use permil and compose multiplicatively.
func ActivityRefreshGoldCost(base int32, effects []*papi.ActivityCampaignEffect) (int64, error) {
	cost := int64(base)
	for _, effect := range effects {
		if effect.GetType() != penum.ActivityCampaignEffectType_ActivityCampaignEffectType_RefreshGoldConsumptionDown {
			continue
		}
		if effect.GetValue() < 0 || effect.GetValue() > 1000 {
			return 0, status.Error(codes.FailedPrecondition, "refresh gold campaign coefficient is invalid")
		}
		cost = cost * int64(effect.GetValue()) / 1000
	}
	return cost, nil
}

// ReconcileActivityCharacterStamina applies time-based stamina recovery using
// current master settings and persists the post-image when it changes.
func ReconcileActivityCharacterStamina(
	ctx context.Context,
	suid string,
	character *ptransaction.UserCharacter,
	now int64,
) (bool, error) {
	if character == nil || character.GetInActivity() || now <= 0 {
		return false, nil
	}
	level := character.GetActivityLevel()
	if level <= 0 {
		level = 1
	}
	activityLevel, err := opmaster.FindCharacterActivityLevel(ctx, int64(level))
	if err != nil {
		return false, err
	}
	maxStamina := activityLevel.GetMaxStamina()
	if character.GetActivityStamina() >= maxStamina {
		return false, nil
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return false, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return false, err
	}
	seconds := setting.GetCharacterSecondToRecover()
	if seconds <= 0 || character.GetStaminaUpdatedTime() <= 0 || now <= character.GetStaminaUpdatedTime() {
		return false, nil
	}
	recovered := (now - character.GetStaminaUpdatedTime()) / (int64(seconds) * 1000)
	if recovered <= 0 {
		return false, nil
	}
	before := character.GetActivityStamina()
	character.ActivityStamina = int32(min(int64(maxStamina), int64(before)+recovered))
	if character.ActivityStamina >= maxStamina {
		character.StaminaUpdatedTime = now
	} else {
		character.StaminaUpdatedTime += recovered * int64(seconds) * 1000
	}
	return true, opuser.UpsertUserCharacter(ctx, suid, character)
}

// ReconcileActivityLessonProgress updates the pending idle rewards and resets
// the daily promotion counter at the JST day boundary.
func ReconcileActivityLessonProgress(ctx context.Context, suid string, now int64) (*papi.ActivityLessonProgress, error) {
	progress, err := opuser.FindUserLessonProgress(ctx, suid)
	if err != nil {
		return nil, err
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return nil, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return nil, err
	}
	changed := false
	day := time.UnixMilli(now).In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02")
	daily, err := opuser.FindUserActivityLessonDailyState(ctx, suid)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		daily = &umodel.UserActivityLessonDailyState{SUID: suid}
	}
	if daily.Day != day {
		daily.Day = day
		progress.DailyPromoteCount = 0
		if err := opuser.UpsertUserActivityLessonDailyState(ctx, daily); err != nil {
			return nil, err
		}
		changed = true
	}
	stepMillis := int64(setting.GetSecondsPerActivityLessonStep()) * 1000
	if stepMillis <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "lesson step duration is invalid")
	}
	if progress.GetLastReceiveTime() <= 0 {
		progress.LastReceiveTime = now
		progress.MaxRewardTime = now + 12*time.Hour.Milliseconds()
		changed = true
	}
	end := min(now, progress.GetMaxRewardTime())
	steps := int64(0)
	if end > progress.GetLastReceiveTime() {
		steps = (end - progress.GetLastReceiveTime()) / stepMillis
	}
	gold, err := checkedActivityAmount(steps, progress.GetStepRewardGold())
	if err != nil {
		return nil, err
	}
	managerExp, err := checkedActivityAmount(steps, progress.GetStepRewardManagerExp())
	if err != nil {
		return nil, err
	}
	items, err := checkedActivityAmount(steps, progress.GetStepRewardCardEnhanceItem())
	if err != nil {
		return nil, err
	}
	if progress.GetRewardGoldAmount() != gold || progress.GetRewardManagerExpAmount() != managerExp ||
		progress.GetRewardCardEnhanceItemAmount() != items {
		progress.RewardGoldAmount = gold
		progress.RewardManagerExpAmount = managerExp
		progress.RewardCardEnhanceItemAmount = items
		changed = true
	}
	if changed {
		if err := opuser.UpsertUserLessonProgress(ctx, suid, progress); err != nil {
			return nil, err
		}
	}
	return progress, nil
}

// ReconcileActivityLessonInfo loads the per-user lesson campaign state and
// removes expired effects. Users created before this state was introduced are
// repaired with an empty record instead of failing Home or reward collection.
func ReconcileActivityLessonInfo(ctx context.Context, suid string, now int64) (*papi.LessonInfo, error) {
	info, err := opuser.FindUserLessonInfo(ctx, suid)
	created := false
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		info = &papi.LessonInfo{
			CampaignEffects:       []*papi.ActivityCampaignEffect{},
			CampaignDropRewardIds: []string{},
		}
		created = true
	}
	live := LiveActivityCampaignEffects(info.GetCampaignEffects(), now)
	if created || len(live) != len(info.GetCampaignEffects()) {
		info.CampaignEffects = live
		if err := opuser.UpsertUserLessonInfo(ctx, suid, info); err != nil {
			return nil, err
		}
	}
	return info, nil
}

func checkedActivityAmount(steps int64, perStep int32) (int32, error) {
	if steps < 0 || perStep < 0 || (perStep > 0 && steps > math.MaxInt32/int64(perStep)) {
		return 0, status.Error(codes.FailedPrecondition, "activity reward amount overflows")
	}
	return int32(steps * int64(perStep)), nil
}
