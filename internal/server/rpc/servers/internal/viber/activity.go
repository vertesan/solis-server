package viber

import (
	"context"
	"errors"
	"math"
	"slices"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const activityLineupSize = 3

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

// ActivityCampaignBonusAmount applies additive permil reward bonuses from all
// live campaign effects of the requested type.
func ActivityCampaignBonusAmount(
	base int64,
	effectType penum.ActivityCampaignEffectType,
	effects []*papi.ActivityCampaignEffect,
) (int64, error) {
	coefficient := int64(1000)
	for _, effect := range effects {
		if effect.GetType() != effectType {
			continue
		}
		if effect.GetValue() < 0 || coefficient > math.MaxInt64-int64(effect.GetValue()) {
			return 0, status.Error(codes.FailedPrecondition, "activity campaign coefficient is invalid")
		}
		coefficient += int64(effect.GetValue())
	}
	if base <= 0 || coefficient == 1000 {
		return base, nil
	}
	if base > math.MaxInt64/coefficient {
		return 0, status.Error(codes.FailedPrecondition, "activity campaign reward overflows")
	}
	return base * coefficient / 1000, nil
}

// ValidateActivityLineup resolves and validates the three parallel character,
// costume and hair selections shared by the timed idol activities.
func ValidateActivityLineup(
	ctx context.Context,
	suid string,
	activityPerformanceID string,
	characterIDs, costumeIDs, hairIDs []string,
	now int64,
) ([]*ptransaction.UserCharacter, []*papi.ActivityCharacterInfo, error) {
	if len(characterIDs) != activityLineupSize || len(costumeIDs) != activityLineupSize || len(hairIDs) != activityLineupSize {
		return nil, nil, status.Errorf(codes.InvalidArgument, "activity lineup must contain exactly %d characters", activityLineupSize)
	}
	performance, err := opmaster.FindActivityPerformance(ctx, activityPerformanceID)
	if err != nil {
		return nil, nil, err
	}
	allowedCostumeTypes := performance.GetCostumeTypeIds()
	if len(allowedCostumeTypes) == 0 && performance.GetForceCostumeTypeId() != "" {
		allowedCostumeTypes = []string{performance.GetForceCostumeTypeId()}
	}
	seen := make(map[string]struct{}, activityLineupSize)
	characters := make([]*ptransaction.UserCharacter, 0, activityLineupSize)
	infos := make([]*papi.ActivityCharacterInfo, 0, activityLineupSize)
	for index, characterID := range characterIDs {
		if characterID == "" || costumeIDs[index] == "" || hairIDs[index] == "" {
			return nil, nil, status.Error(codes.InvalidArgument, "character, costume and hair ids are required")
		}
		if _, duplicate := seen[characterID]; duplicate {
			return nil, nil, status.Error(codes.InvalidArgument, "activity characters must be unique")
		}
		seen[characterID] = struct{}{}
		character, err := opuser.FindUserCharacter(ctx, suid, characterID)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, nil, status.Error(codes.PermissionDenied, "character is not owned")
			}
			return nil, nil, err
		}
		if character.GetInActivity() {
			return nil, nil, status.Error(codes.FailedPrecondition, "character is already in an activity")
		}
		if _, err := ReconcileActivityCharacterStamina(ctx, suid, character, now); err != nil {
			return nil, nil, err
		}
		if _, err := opuser.FindUserCostume(ctx, suid, costumeIDs[index]); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, nil, status.Error(codes.PermissionDenied, "costume is not owned")
			}
			return nil, nil, err
		}
		if _, err := opuser.FindUserHair(ctx, suid, hairIDs[index]); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, nil, status.Error(codes.PermissionDenied, "hair is not owned")
			}
			return nil, nil, err
		}
		costume, err := opmaster.FindCostume(ctx, costumeIDs[index])
		if err != nil {
			return nil, nil, err
		}
		hair, err := opmaster.FindHair(ctx, hairIDs[index])
		if err != nil {
			return nil, nil, err
		}
		if costume.GetCharacterId() != characterID || hair.GetCharacterId() != characterID {
			return nil, nil, status.Error(codes.InvalidArgument, "costume or hair belongs to another character")
		}
		if len(allowedCostumeTypes) > 0 && !slices.Contains(allowedCostumeTypes, costume.GetCostumeTypeId()) {
			return nil, nil, status.Error(codes.InvalidArgument, "costume type is unavailable for this activity")
		}
		if slices.Contains(hair.GetNotWearableCostumeIds(), costumeIDs[index]) ||
			(len(hair.GetWearableCostumeIds()) > 0 && !slices.Contains(hair.GetWearableCostumeIds(), costumeIDs[index])) {
			return nil, nil, status.Error(codes.InvalidArgument, "hair cannot be worn with costume")
		}
		characters = append(characters, character)
		infos = append(infos, &papi.ActivityCharacterInfo{
			CharacterId: characterID,
			CostumeId:   costumeIDs[index],
			HairId:      hairIDs[index],
		})
	}
	return characters, infos, nil
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

// GrantActivityReward applies the resource kinds emitted by timed activities.
// Manager EXP is not a normal inventory resource, so it is handled here while
// all other supported resources reuse the common grant path.
func GrantActivityReward(
	ctx context.Context,
	suid string,
	reward *ptransaction.Reward,
	updated *papi.UpdatedData,
) error {
	if reward == nil || reward.GetAmount() <= 0 {
		return status.Error(codes.FailedPrecondition, "activity reward must be positive")
	}
	if reward.GetResourceType() == penum.ResourceType_ResourceType_ManagerExp {
		_, err := AddManagerExp(ctx, suid, reward.GetAmount(), updated)
		return err
	}
	if reward.GetAmount() > math.MaxInt32 {
		return status.Error(codes.FailedPrecondition, "activity reward amount exceeds supported range")
	}
	_, err := GrantExchangeRewards(ctx, suid, &pmaster.RewardResource{
		ResourceType: reward.GetResourceType(),
		ResourceId:   reward.GetResourceId(),
		Amount:       int32(reward.GetAmount()),
	}, updated)
	return err
}

// RecalculateAudienceCandidateAmount keeps the live audience projection in
// sync after an activity changes the three fan tiers. The official service
// floors each weighted tier independently before summing them.
func RecalculateAudienceCandidateAmount(ctx context.Context, character *ptransaction.UserCharacter) error {
	if character == nil {
		return status.Error(codes.FailedPrecondition, "activity character is missing")
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return err
	}
	amount := int64(character.GetLightFanAmount())*int64(setting.GetLiveLightFanAmountWeightPermil())/1000 +
		int64(character.GetMiddleFanAmount())*int64(setting.GetLiveMiddleFanAmountWeightPermil())/1000 +
		int64(character.GetHeavyFanAmount())*int64(setting.GetLiveHeavyFanAmountWeightPermil())/1000
	if amount < 0 || amount > math.MaxInt32 {
		return status.Error(codes.FailedPrecondition, "activity audience candidate amount is invalid")
	}
	character.AudienceCandidateAmount = int32(amount)
	return nil
}

func AddManagerExp(
	ctx context.Context,
	suid string,
	amount int64,
	updated *papi.UpdatedData,
) (*ptransaction.User, error) {
	if amount <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "manager exp must be positive")
	}
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, err
	}
	if user.GetManagerExp() > math.MaxInt64-amount {
		return nil, status.Error(codes.FailedPrecondition, "manager exp overflows")
	}
	user.ManagerExp += amount
	maxLevel, err := opmaster.FindMaxManagerLevel(ctx)
	if err != nil {
		return nil, err
	}
	for user.GetManagerLevel() < maxLevel.GetLevel() {
		next, err := opmaster.FindManagerLevel(ctx, user.GetManagerLevel()+1)
		if err != nil {
			return nil, err
		}
		if user.GetManagerExp() < next.GetRequiredExp() {
			break
		}
		user.ManagerLevel = next.GetLevel()
	}
	if err := opuser.UpsertUser(ctx, suid, user); err != nil {
		return nil, err
	}
	updated.User = user
	unlockedPhotos, err := UnlockManagerLevelPhotoAbilities(ctx, suid, user.GetManagerLevel())
	if err != nil {
		return nil, err
	}
	updated.Photos = append(updated.Photos, unlockedPhotos...)
	return user, nil
}

func ActivityRewardFromMaster(ctx context.Context, rewardID string) ([]*ptransaction.Reward, error) {
	if rewardID == "" {
		return nil, nil
	}
	resources, err := ParseAutoReward(rewardID)
	if err != nil {
		return nil, err
	}
	if resources == nil {
		master, err := opmaster.FindReward(ctx, rewardID)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Errorf(codes.FailedPrecondition, "activity reward %q is missing", rewardID)
			}
			return nil, err
		}
		resources = master.GetResources()
	}
	rewards := make([]*ptransaction.Reward, 0, len(resources))
	for _, resource := range resources {
		if resource == nil {
			return nil, status.Errorf(codes.FailedPrecondition, "activity reward %q is invalid", rewardID)
		}
		// Fan-event reward masters use zero amounts as display-time
		// placeholders for rewards whose quantities are calculated from the
		// completed run. They are not literal negative/zero grants; omit them
		// here while preserving every fixed positive resource in the catalog.
		if resource.GetAmount() == 0 {
			continue
		}
		if resource.GetAmount() < 0 {
			return nil, status.Errorf(codes.FailedPrecondition, "activity reward %q is invalid", rewardID)
		}
		rewards = append(rewards, &ptransaction.Reward{
			ResourceType: resource.GetResourceType(),
			ResourceId:   resource.GetResourceId(),
			Amount:       int64(resource.GetAmount()),
		})
	}
	return rewards, nil
}
