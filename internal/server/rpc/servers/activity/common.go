package activity

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/salary"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.yaml.in/yaml/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const activityLineupSize = 3

// activityCampaignBonusAmount applies additive permil reward bonuses from all
// live campaign effects of the requested type.
func activityCampaignBonusAmount(
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

// validateActivityLineup resolves and validates the three parallel character,
// costume and hair selections shared by the timed idol activities.
func validateActivityLineup(
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
		if _, err := viber.ReconcileActivityCharacterStamina(ctx, suid, character, now); err != nil {
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

// grantActivityReward applies the resource kinds emitted by timed activities.
// Manager EXP is not a normal inventory resource, so it is handled here while
// all other supported resources reuse the common grant path.
func grantActivityReward(
	ctx context.Context,
	suid string,
	reward *ptransaction.Reward,
	updated *papi.UpdatedData,
) error {
	if reward == nil || reward.GetAmount() <= 0 {
		return status.Error(codes.FailedPrecondition, "activity reward must be positive")
	}
	if reward.GetResourceType() == penum.ResourceType_ResourceType_ManagerExp {
		_, err := viber.AddManagerExp(ctx, suid, reward.GetAmount(), updated)
		return err
	}
	if reward.GetAmount() > math.MaxInt32 {
		return status.Error(codes.FailedPrecondition, "activity reward amount exceeds supported range")
	}
	_, err := viber.GrantExchangeRewards(ctx, suid, &pmaster.RewardResource{
		ResourceType: reward.GetResourceType(),
		ResourceId:   reward.GetResourceId(),
		Amount:       int32(reward.GetAmount()),
	}, updated)
	return err
}

// recalculateAudienceCandidateAmount keeps the live audience projection in
// sync after an activity changes the three fan tiers. The official service
// floors each weighted tier independently before summing them.
func recalculateAudienceCandidateAmount(ctx context.Context, character *ptransaction.UserCharacter) error {
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

func maxUnlockedPromotionLevel(ctx context.Context, suid, promotionID string, now int64, fallback int32) (int32, error) {
	masters, err := opmaster.FindAllActivityPromotions(ctx)
	if err != nil {
		return 0, err
	}
	evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now)
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

func issueFanEventSteps(
	ctx context.Context,
	progress *papi.FanEventProgress,
	state *umodel.UserFanEventRunState,
	cursors []int32,
) ([]*papi.FanEventCharacterSteps, error) {
	if len(cursors) != len(progress.GetCharacters()) {
		return nil, status.Error(codes.InvalidArgument, "one step cursor is required per character")
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return nil, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return nil, err
	}
	fetchSize := setting.GetActivityFanEventFetchStepsSize()
	if fetchSize <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "fan event fetch size is invalid")
	}
	superModeValue, err := viber.FanEventSuperModeValue(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*papi.FanEventCharacterSteps, 0, len(progress.GetCharacters()))
	for index, character := range progress.GetCharacters() {
		cursor := cursors[index]
		if cursor < 0 || cursor > character.GetMaxStep() {
			return nil, status.Error(codes.InvalidArgument, "fan event step cursor is out of range")
		}
		first := max(int32(1), cursor)
		last := min(character.GetMaxStep(), cursor+fetchSize)
		if cursor == 0 {
			last = min(character.GetMaxStep(), fetchSize)
		}
		if err := viber.EnsureFanEventPlan(state, progress, character.GetCharacterId(), last); err != nil {
			return nil, err
		}
		steps := make([]*papi.FanEventStep, 0)
		for step := first; step <= last; step++ {
			issued := &papi.FanEventStep{Step: step, Happenings: []*papi.FanEventHappening{}, ItemEffects: []*papi.FanEventItemEffect{}}
			if viber.FanEventStepHasSpecialFan(state, character.GetCharacterId(), step) {
				issued.Happenings = append(issued.Happenings, &papi.FanEventHappening{Type: penum.ActivityFanEventHappeningType_ActivityFanEventHappeningType_SpecialFan})
			}
			if cheer := viber.FanEventEffectAt(state.CheerEffects, character.GetCharacterId(), step); cheer != nil {
				issued.Happenings = append(issued.Happenings, &papi.FanEventHappening{
					Type: penum.ActivityFanEventHappeningType_ActivityFanEventHappeningType_Cheer, CharacterId: cheer.SupportCharacterId,
				})
			}
			if viber.FanEventStepHasSuperMode(state, character.GetCharacterId(), step) {
				issued.ItemEffects = append(issued.ItemEffects, &papi.FanEventItemEffect{
					Type: penum.ItemType_ItemType_ActivityFanEventSuperMode, Value: superModeValue,
				})
			}
			if len(issued.GetHappenings()) > 0 || len(issued.GetItemEffects()) > 0 {
				steps = append(steps, issued)
			}
		}
		result = append(result, &papi.FanEventCharacterSteps{CharacterId: character.GetCharacterId(), Steps: steps})
	}
	return result, nil
}

type fanEventBalanceCatalog struct {
	Profiles                    []FanEventBalanceProfile `yaml:"profiles"`
	CoreFanConversionMillionths map[string]int32         `yaml:"coreFanConversionMillionths"`
	Reward                      FanEventRewardCatalog    `yaml:"reward"`
	RewardProfiles              []FanEventRewardProfile  `yaml:"rewardProfiles"`
}

type FanEventBalanceProfile struct {
	ID                             string           `yaml:"id"`
	Level                          int32            `yaml:"level"`
	ActivityPointCoefficientPermil int32            `yaml:"activityPointCoefficientPermil"`
	RankAThresholdPoint            int64            `yaml:"rankAThresholdPoint"`
	CharacterActivityExp           int64            `yaml:"characterActivityExp"`
	CoreFanConversionMillionths    map[string]int32 `yaml:"coreFanConversionMillionths"`
}

type FanEventRewardCatalog struct {
	RecipeDropChancePermyriads int32            `yaml:"recipeDropChancePermyriads"`
	RankCoefficientsPermyriads map[string]int32 `yaml:"rankCoefficientsPermyriads"`
	ManagerExp                 map[string]int64 `yaml:"managerExp"`
}

type FanEventRewardProfile struct {
	ID            string                           `yaml:"id"`
	Level         int32                            `yaml:"level"`
	DynamicByRank map[string]FanEventDynamicReward `yaml:"dynamicByRank"`
}

type FanEventDynamicReward struct {
	CardEnhance int64 `yaml:"cardEnhance"`
	CardSkill   int64 `yaml:"cardSkill"`
}

//go:embed catalogdata/fan_event_profiles.yaml
var fanEventBalanceFiles embed.FS

var (
	loadFanEventBalanceOnce sync.Once
	loadedFanEventBalance   *fanEventBalanceCatalog
	loadFanEventBalanceErr  error
)

func findFanEventBalanceProfile(id string, level int32) (*FanEventBalanceProfile, error) {
	loadFanEventBalanceOnce.Do(func() {
		data, err := fanEventBalanceFiles.ReadFile("catalogdata/fan_event_profiles.yaml")
		if err != nil {
			loadFanEventBalanceErr = fmt.Errorf("read fan event balance catalog: %w", err)
			return
		}
		catalog := new(fanEventBalanceCatalog)
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(catalog); err != nil {
			loadFanEventBalanceErr = fmt.Errorf("decode fan event balance catalog: %w", err)
			return
		}
		for _, profile := range catalog.Profiles {
			if profile.ID == "" || profile.Level <= 0 || profile.ActivityPointCoefficientPermil <= 0 ||
				profile.RankAThresholdPoint <= 0 || profile.CharacterActivityExp <= 0 {
				loadFanEventBalanceErr = fmt.Errorf("fan event balance catalog contains an invalid profile")
				return
			}
			for rank, coefficient := range profile.CoreFanConversionMillionths {
				if !validFanEventRankKey(rank) || coefficient <= 0 {
					loadFanEventBalanceErr = fmt.Errorf("fan event profile core-fan coefficient %q is invalid", rank)
					return
				}
			}
		}
		for _, key := range fanEventRankKeys {
			if catalog.CoreFanConversionMillionths[key] <= 0 {
				loadFanEventBalanceErr = fmt.Errorf("fan event core-fan rank %q is invalid", key)
				return
			}
		}
		if catalog.Reward.RecipeDropChancePermyriads < 0 || catalog.Reward.RecipeDropChancePermyriads > 10000 {
			loadFanEventBalanceErr = fmt.Errorf("fan event reward drop chance is invalid")
			return
		}
		for _, key := range fanEventRankKeys {
			if catalog.Reward.RankCoefficientsPermyriads[key] <= 0 || catalog.Reward.ManagerExp[key] <= 0 {
				loadFanEventBalanceErr = fmt.Errorf("fan event reward rank %q is invalid", key)
				return
			}
		}
		for _, profile := range catalog.RewardProfiles {
			if profile.ID == "" || profile.Level <= 0 {
				loadFanEventBalanceErr = fmt.Errorf("fan event dynamic reward profile is invalid")
				return
			}
			for _, reward := range profile.DynamicByRank {
				if reward.CardEnhance <= 0 || reward.CardSkill <= 0 {
					loadFanEventBalanceErr = fmt.Errorf("fan event dynamic reward amount is invalid")
					return
				}
			}
		}
		loadedFanEventBalance = catalog
	})
	if loadFanEventBalanceErr != nil {
		return nil, loadFanEventBalanceErr
	}
	for index := range loadedFanEventBalance.Profiles {
		profile := &loadedFanEventBalance.Profiles[index]
		if profile.ID == id && profile.Level == level {
			return profile, nil
		}
	}
	return nil, nil
}

var fanEventRankKeys = []string{"D", "C", "B", "A", "S", "Splus"}

func validFanEventRankKey(candidate string) bool {
	for _, key := range fanEventRankKeys {
		if candidate == key {
			return true
		}
	}
	return false
}

func findUserFanEvent(info *papi.FanEventInfo, id string) *papi.FanEvent {
	for _, fanEvent := range info.GetFanEvents() {
		if fanEvent.GetId() == id {
			return fanEvent
		}
	}
	return nil
}

func syncActiveFanEvent(ctx context.Context, suid string, progress *papi.FanEventProgress) error {
	info, err := opuser.FindUserFanEventInfo(ctx, suid)
	if err != nil {
		return err
	}
	viber.SyncActiveFanEventInfo(info, progress)
	return opuser.UpsertUserFanEventInfo(ctx, suid, info)
}

func fanEventScore(progress *papi.FanEventProgress) int64 {
	var score int64
	for _, character := range progress.GetCharacters() {
		score += int64(character.GetActivityPoint())
	}
	return score
}

func fanEventRank(patterns []*papi.FanEventRankPattern, score int64) (penum.ResultRankType, int32) {
	for _, pattern := range patterns {
		if score >= int64(pattern.GetThresholdPoint()) {
			return pattern.GetRankType(), pattern.GetPlus()
		}
	}
	return penum.ResultRankType_ResultRankType_D, 0
}

func updateFanEventRanking(ctx context.Context, suid string, progress *papi.FanEventProgress) error {
	ranking, err := opuser.FindUserFanEventRanking(ctx, suid, progress.GetActivityFanEventId())
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		ranking = &umodel.UserFanEventRanking{SUID: suid, ActivityFanEventId: progress.GetActivityFanEventId()}
	}
	score := fanEventScore(progress)
	if score <= ranking.BestScore {
		return nil
	}
	ranking.BestScore = score
	ranking.CharacterIds = ranking.CharacterIds[:0]
	for _, character := range progress.GetCharacters() {
		ranking.CharacterIds = append(ranking.CharacterIds, character.GetCharacterId())
	}
	if err := opuser.UpsertUserFanEventRanking(ctx, ranking); err != nil {
		return err
	}
	return salary.RefreshSnapshot(ctx, suid, time.Now())
}

const lessonCardEnhanceItemID = "item-card_enhance"

// advanceLessonReceiveAnchor advances across every complete elapsed step while
// preserving the sub-step remainder. Reward reconciliation separately caps
// the paid interval at twelve hours; advancing against now discards excess
// idle time instead of letting repeated receives bypass that cap. A promotion
// with no pending idle steps leaves both timestamps unchanged and only grants
// its explicit bonus window.
func advanceLessonReceiveAnchor(progress *papi.ActivityLessonProgress, now int64, stepSeconds int32) error {
	if progress == nil || stepSeconds <= 0 {
		return status.Error(codes.FailedPrecondition, "lesson step duration is invalid")
	}
	stepMillis := int64(stepSeconds) * 1000
	if now <= progress.GetLastReceiveTime() {
		return nil
	}
	steps := (now - progress.GetLastReceiveTime()) / stepMillis
	if steps <= 0 {
		return nil
	}
	if steps > (math.MaxInt64-progress.GetLastReceiveTime())/stepMillis {
		return status.Error(codes.FailedPrecondition, "lesson receive time overflows")
	}
	progress.LastReceiveTime += steps * stepMillis
	window := int64(12 * time.Hour / time.Millisecond)
	if progress.GetLastReceiveTime() > math.MaxInt64-window {
		return status.Error(codes.FailedPrecondition, "lesson reward window overflows")
	}
	progress.MaxRewardTime = progress.GetLastReceiveTime() + window
	return nil
}

func receiveLessonRewards(
	ctx context.Context,
	suid string,
	progress *papi.ActivityLessonProgress,
	bonusSteps int64,
	effects []*papi.ActivityCampaignEffect,
	updated *papi.UpdatedData,
) ([]*ptransaction.Reward, error) {
	amounts := []struct {
		resourceType penum.ResourceType
		resourceID   string
		pending      int64
		perStep      int32
		effectType   penum.ActivityCampaignEffectType
	}{
		{
			penum.ResourceType_ResourceType_Gold,
			"",
			int64(progress.GetRewardGoldAmount()),
			progress.GetStepRewardGold(),
			penum.ActivityCampaignEffectType_ActivityCampaignEffectType_LessonGoldUp,
		},
		{
			penum.ResourceType_ResourceType_ManagerExp,
			"",
			int64(progress.GetRewardManagerExpAmount()),
			progress.GetStepRewardManagerExp(),
			penum.ActivityCampaignEffectType_ActivityCampaignEffectType_LessonExpUp,
		},
		{
			penum.ResourceType_ResourceType_Item,
			lessonCardEnhanceItemID,
			int64(progress.GetRewardCardEnhanceItemAmount()),
			progress.GetStepRewardCardEnhanceItem(),
			penum.ActivityCampaignEffectType_ActivityCampaignEffectType_LessonCardEnhanceItemUp,
		},
	}
	rewards := make([]*ptransaction.Reward, 0, len(amounts))
	for _, value := range amounts {
		if bonusSteps > 0 && value.perStep > 0 {
			if bonusSteps > math.MaxInt64/int64(value.perStep) {
				return nil, status.Error(codes.FailedPrecondition, "lesson reward amount overflows")
			}
			bonus := bonusSteps * int64(value.perStep)
			if value.pending > math.MaxInt64-bonus {
				return nil, status.Error(codes.FailedPrecondition, "lesson reward amount overflows")
			}
			value.pending += bonus
		}
		adjusted, err := activityCampaignBonusAmount(value.pending, value.effectType, effects)
		if err != nil {
			return nil, err
		}
		value.pending = adjusted
		if value.pending <= 0 {
			continue
		}
		reward := &ptransaction.Reward{
			ResourceType: value.resourceType,
			ResourceId:   value.resourceID,
			Amount:       value.pending,
		}
		if err := grantActivityReward(ctx, suid, reward, updated); err != nil {
			return nil, err
		}
		rewards = append(rewards, reward)
	}
	return rewards, nil
}

type promotionSettlement struct {
	rewards       []*ptransaction.Reward
	levelProgress *papi.ActivityLevelProgress
	updated       *papi.UpdatedData
}

func settlePromotionRewards(
	ctx context.Context,
	suid string,
	progress *papi.PromotionProgress,
	characters []*ptransaction.UserCharacter,
	finish bool,
) (*promotionSettlement, error) {
	updated := &papi.UpdatedData{}
	runState, err := opuser.FindUserPromotionRunState(ctx, suid)
	if err != nil {
		return nil, err
	}
	if !runState.AccountingInitialized {
		// Compatibility for a run created before the accounting sidecar was
		// introduced. Pending gold is only used once to reconstruct the old
		// settlement cursor; every subsequent claim uses completed steps.
		legacyPendingSteps := int64(0)
		if progress.GetStepRewardGold() > 0 {
			legacyPendingSteps = int64(progress.GetRewardGoldAmount()) / int64(progress.GetStepRewardGold())
		}
		runState.LastSettledCompletedStep = max(int64(0), int64(progress.GetCompletedStep())-legacyPendingSteps)
		runState.FanRewardStepRemainder = runState.LastSettledCompletedStep % 20
		runState.AccountingInitialized = true
	}
	if runState.LastSettledCompletedStep < 0 || runState.LastSettledCompletedStep > int64(progress.GetCompletedStep()) {
		return nil, status.Error(codes.FailedPrecondition, "promotion settlement cursor is invalid")
	}
	pendingSteps := int64(progress.GetCompletedStep()) - runState.LastSettledCompletedStep
	info, err := opuser.FindUserPromotionInfo(ctx, suid)
	if err != nil {
		return nil, err
	}
	promotion := activityPromotionByID(info, progress.GetActivityPromotionId())
	if promotion == nil {
		return nil, status.Error(codes.FailedPrecondition, "active promotion definition is missing")
	}
	finishedAt := time.Now().UnixMilli()
	maxLevel, err := maxUnlockedPromotionLevel(ctx, suid, promotion.GetId(), finishedAt, promotion.GetLevel())
	if err != nil {
		return nil, err
	}
	before := &papi.ActivityLevelInfo{
		Exp:             promotion.GetExp(),
		Level:           promotion.GetLevel(),
		NextLevelLocked: promotionNextLevelLocked(promotion, promotion.GetExp()),
	}
	activityExpGain := pendingSteps * int64(progress.GetStepRewardActivityExp())
	promotion.Exp = promotionInt32(int64(promotion.GetExp()) + activityExpGain)
	after := &papi.ActivityLevelInfo{
		Exp:             promotion.GetExp(),
		Level:           promotion.GetLevel(),
		NextLevelLocked: promotionNextLevelLocked(promotion, promotion.GetExp()),
	}
	if err := opuser.UpsertUserPromotionInfo(ctx, suid, info); err != nil {
		return nil, err
	}
	fanRewardSteps := runState.FanRewardStepRemainder + pendingSteps
	fanGain := fanRewardSteps / 20
	runState.FanRewardStepRemainder = fanRewardSteps % 20
	fanGain, err = activityCampaignBonusAmount(
		fanGain,
		penum.ActivityCampaignEffectType_ActivityCampaignEffectType_PromotionFanUp,
		progress.GetCampaignEffects(),
	)
	if err != nil {
		return nil, err
	}
	for _, character := range characters {
		characterExpGain := pendingSteps * int64(progress.GetStepRewardCharacterActivityExp())
		if characterExpGain > 0 && character.GetActivityExp() > math.MaxInt64-characterExpGain {
			return nil, status.Error(codes.FailedPrecondition, "character activity exp overflows")
		}
		character.ActivityExp += characterExpGain
		if err := addPromotionFans(character, promotion.GetMaxFanAmount(), fanGain); err != nil {
			return nil, err
		}
		if err := recalculateAudienceCandidateAmount(ctx, character); err != nil {
			return nil, err
		}
		if finish {
			character.InActivity = false
			character.StaminaUpdatedTime = finishedAt
		}
		if err := updateCharacterActivityLevel(ctx, character); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
			return nil, err
		}
	}
	updated.Characters = characters
	rewards := make([]*ptransaction.Reward, 0, 3)
	managerExp := pendingSteps * int64(progress.GetStepRewardManagerExp())
	if managerExp > 0 {
		reward := &ptransaction.Reward{ResourceType: penum.ResourceType_ResourceType_ManagerExp, Amount: managerExp}
		if err := grantActivityReward(ctx, suid, reward, updated); err != nil {
			return nil, err
		}
		rewards = append(rewards, reward)
	}
	gold := int64(progress.GetRewardGoldAmount())
	if gold > 0 {
		reward := &ptransaction.Reward{ResourceType: penum.ResourceType_ResourceType_Gold, Amount: gold}
		if err := grantActivityReward(ctx, suid, reward, updated); err != nil {
			return nil, err
		}
		rewards = append(rewards, reward)
	}
	cardEnhance := int64(progress.GetRewardCardEnhanceItemAmount())
	if cardEnhance > 0 {
		reward := &ptransaction.Reward{
			ResourceType: penum.ResourceType_ResourceType_Item,
			ResourceId:   "item-card_enhance",
			Amount:       cardEnhance,
		}
		if err := grantActivityReward(ctx, suid, reward, updated); err != nil {
			return nil, err
		}
		rewards = append(rewards, reward)
	}
	dropRewards, err := settlePromotionDropRewards(ctx, suid, progress.GetRewards(), updated)
	if err != nil {
		return nil, err
	}
	rewards = append(rewards, dropRewards...)
	if pendingSteps > 0 {
		settingID, err := config.GetGameSettingId()
		if err != nil {
			return nil, err
		}
		setting, err := opmaster.FindSetting(ctx, settingID)
		if err != nil {
			return nil, err
		}
		totalCount, err := opuser.FindUserTotalCount(ctx, suid)
		if err != nil {
			return nil, err
		}
		minutes := pendingSteps * int64(setting.GetSecondsPerActivityPromotionStep()) / 60
		if minutes > 0 && totalCount.GetActivityPromotionMinutes() > math.MaxInt64-minutes {
			return nil, status.Error(codes.FailedPrecondition, "activity promotion minutes overflows")
		}
		totalCount.ActivityPromotionMinutes += minutes
		if err := opuser.UpsertUserTotalCount(ctx, suid, totalCount); err != nil {
			return nil, err
		}
		updated.TotalCount = totalCount
	}
	progress.RewardGoldAmount = 0
	progress.RewardCardEnhanceItemAmount = 0
	progress.Rewards = []*papi.ActivityPromotionReward{}
	runState.LastSettledCompletedStep = int64(progress.GetCompletedStep())
	if err := opuser.UpsertUserPromotionRunState(ctx, runState); err != nil {
		return nil, err
	}
	if err := salary.RefreshSnapshot(ctx, suid, time.Now()); err != nil {
		return nil, err
	}
	return &promotionSettlement{
		rewards: rewards,
		levelProgress: &papi.ActivityLevelProgress{
			MaxActivityLevel: maxLevel,
			Before:           before,
			After:            after,
		},
		updated: updated,
	}, nil
}

func promotionNextLevelLocked(promotion *papi.Promotion, exp int32) bool {
	return promotion != nil && promotion.GetNextLevelRequiredExp() > 0 &&
		exp >= promotion.GetNextLevelRequiredExp() && !promotion.GetIsNextLevelUnlocked()
}

func advancePromotion(ctx context.Context, suid string, progress *papi.PromotionProgress, now int64) ([]*ptransaction.UserCharacter, error) {
	if progress == nil || len(progress.GetCharacters()) != 3 || progress.GetStepConsumptionStamina() <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "active promotion data is invalid")
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return nil, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return nil, err
	}
	stepMillis := int64(setting.GetSecondsPerActivityPromotionStep()) * 1000
	if stepMillis <= 0 || progress.GetFinishTime() < progress.GetStartTime() {
		return nil, status.Error(codes.FailedPrecondition, "promotion timing is invalid")
	}
	maxSteps := (progress.GetFinishTime() - progress.GetStartTime()) / stepMillis
	completed := int64(0)
	if now > progress.GetStartTime() {
		completed = (now - progress.GetStartTime()) / stepMillis
	}
	if completed > maxSteps {
		completed = maxSteps
	}
	if completed > math.MaxInt32 {
		completed = math.MaxInt32
	}
	if completed < int64(progress.GetCompletedStep()) {
		completed = int64(progress.GetCompletedStep())
	}
	characters := make([]*ptransaction.UserCharacter, 0, len(progress.GetCharacters()))
	for _, selected := range progress.GetCharacters() {
		character, err := opuser.FindUserCharacter(ctx, suid, selected.GetCharacterId())
		if err != nil {
			return nil, err
		}
		characters = append(characters, character)
	}
	delta := completed - int64(progress.GetCompletedStep())
	consumption := progress.GetStepConsumptionStamina()
	for step := int64(0); step < delta; step++ {
		start := int((int64(progress.GetCompletedStep()) + step) % int64(len(characters)))
		consumed := false
		for offset := 0; offset < len(characters); offset++ {
			character := characters[(start+offset)%len(characters)]
			if character.ActivityStamina >= consumption {
				character.ActivityStamina -= consumption
				consumed = true
				break
			}
		}
		if !consumed {
			completed = int64(progress.GetCompletedStep()) + step
			delta = step
			break
		}
	}
	if delta > 0 {
		for _, character := range characters {
			character.StaminaUpdatedTime = now
			if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
				return nil, err
			}
		}
		progress.CompletedStep = int32(completed)
		gold, err := promotionRewardDelta(
			completed-delta,
			completed,
			progress.GetStepRewardGold(),
			penum.ActivityCampaignEffectType_ActivityCampaignEffectType_PromotionGoldUp,
			progress.GetCampaignEffects(),
		)
		if err != nil {
			return nil, err
		}
		cardEnhance, err := promotionRewardDelta(
			completed-delta,
			completed,
			progress.GetStepRewardCardEnhanceItem(),
			penum.ActivityCampaignEffectType_ActivityCampaignEffectType_PromotionCardEnhanceItemUp,
			progress.GetCampaignEffects(),
		)
		if err != nil {
			return nil, err
		}
		progress.RewardGoldAmount = promotionInt32(int64(progress.GetRewardGoldAmount()) + gold)
		progress.RewardCardEnhanceItemAmount = promotionInt32(int64(progress.GetRewardCardEnhanceItemAmount()) + cardEnhance)
		if err := appendPromotionMultiStepRewards(ctx, progress, completed-delta, completed); err != nil {
			return nil, err
		}
	}
	return characters, nil
}

func promotionRewardDelta(
	previousStep int64,
	completedStep int64,
	perStep int32,
	effectType penum.ActivityCampaignEffectType,
	effects []*papi.ActivityCampaignEffect,
) (int64, error) {
	if previousStep < 0 || completedStep < previousStep || perStep < 0 {
		return 0, status.Error(codes.FailedPrecondition, "promotion reward interval is invalid")
	}
	if perStep > 0 && completedStep > math.MaxInt64/int64(perStep) {
		return 0, status.Error(codes.FailedPrecondition, "promotion reward amount overflows")
	}
	completed, err := activityCampaignBonusAmount(completedStep*int64(perStep), effectType, effects)
	if err != nil {
		return 0, err
	}
	previous, err := activityCampaignBonusAmount(previousStep*int64(perStep), effectType, effects)
	if err != nil {
		return 0, err
	}
	return completed - previous, nil
}

func addPromotionFans(character *ptransaction.UserCharacter, maxFanAmount int32, amount int64) error {
	if character == nil || maxFanAmount <= 0 || amount <= 0 {
		return nil
	}
	total := int64(character.GetLightFanAmount()) + int64(character.GetMiddleFanAmount()) + int64(character.GetHeavyFanAmount())
	if total < 0 {
		return status.Error(codes.FailedPrecondition, "character fan amount is invalid")
	}
	remaining := int64(maxFanAmount) - total
	if remaining <= 0 {
		return nil
	}
	amount = min(amount, remaining)
	if int64(character.GetLightFanAmount()) > math.MaxInt32-amount {
		return status.Error(codes.FailedPrecondition, "character fan amount overflows")
	}
	character.LightFanAmount += int32(amount)
	return nil
}

func appendPromotionMultiStepRewards(
	ctx context.Context,
	progress *papi.PromotionProgress,
	previousStep int64,
	completedStep int64,
) error {
	if progress.GetMultiStepRewardId() == "" || completedStep/20 <= previousStep/20 {
		return nil
	}
	reward, err := opmaster.FindReward(ctx, progress.GetMultiStepRewardId())
	if err != nil {
		return err
	}
	resources := reward.GetResources()
	if len(resources) == 0 {
		return status.Errorf(codes.FailedPrecondition, "promotion reward %q has no resources", progress.GetMultiStepRewardId())
	}
	for milestone := previousStep/20 + 1; milestone <= completedStep/20; milestone++ {
		index, err := viber.CryptoRandomIndex(len(resources))
		if err != nil {
			return err
		}
		resource := resources[index]
		if resource == nil || resource.GetAmount() < 0 {
			return status.Errorf(codes.FailedPrecondition, "promotion reward %q is invalid", progress.GetMultiStepRewardId())
		}
		amount := int64(resource.GetAmount())
		if amount == 0 {
			amount = 1
		}
		progress.Rewards = append(progress.Rewards, &papi.ActivityPromotionReward{
			ResourceType: resource.GetResourceType(),
			ResourceId:   resource.GetResourceId(),
			Amount:       amount,
		})
	}
	return nil
}

type promotionDropRewardKey struct {
	resourceType penum.ResourceType
	resourceID   string
}

func settlePromotionDropRewards(
	ctx context.Context,
	suid string,
	drops []*papi.ActivityPromotionReward,
	updated *papi.UpdatedData,
) ([]*ptransaction.Reward, error) {
	amounts := make(map[promotionDropRewardKey]int64, len(drops))
	order := make([]promotionDropRewardKey, 0, len(drops))
	for _, drop := range drops {
		if drop == nil || drop.GetAmount() <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "promotion drop reward is invalid")
		}
		key := promotionDropRewardKey{resourceType: drop.GetResourceType(), resourceID: drop.GetResourceId()}
		if _, exists := amounts[key]; !exists {
			order = append(order, key)
		}
		if amounts[key] > math.MaxInt64-drop.GetAmount() {
			return nil, status.Error(codes.FailedPrecondition, "promotion drop reward overflows")
		}
		amounts[key] += drop.GetAmount()
	}
	rewards := make([]*ptransaction.Reward, 0, len(order))
	for _, key := range order {
		reward := &ptransaction.Reward{
			ResourceType: key.resourceType,
			ResourceId:   key.resourceID,
			Amount:       amounts[key],
		}
		if err := grantActivityReward(ctx, suid, reward, updated); err != nil {
			return nil, err
		}
		rewards = append(rewards, reward)
	}
	return rewards, nil
}

func updateCharacterActivityLevel(ctx context.Context, character *ptransaction.UserCharacter) error {
	for {
		next, err := opmaster.FindCharacterActivityLevel(ctx, int64(character.GetActivityLevel()+1))
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil
		}
		if err != nil {
			return err
		}
		if character.GetActivityExp() < int64(next.GetRequiredExp()) {
			return nil
		}
		character.ActivityLevel = next.GetLevel()
	}
}

func activityPromotionByID(info *papi.PromotionInfo, id string) *papi.Promotion {
	for _, promotion := range info.GetPromotions() {
		if promotion.GetId() == id {
			return promotion
		}
	}
	return nil
}

func activityShorteningMillis(value, amount int32) (int64, error) {
	if value <= 0 || amount <= 0 {
		return 0, viber.InvalidArgumentf("shortening value and amount must be positive")
	}
	minutes := int64(value) * int64(amount)
	millisPerMinute := int64(time.Minute / time.Millisecond)
	if minutes > math.MaxInt64/millisPerMinute {
		return 0, status.Error(codes.InvalidArgument, "shortening duration overflows")
	}
	return minutes * millisPerMinute, nil
}

func promotionInt32(value int64) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	if value < math.MinInt32 {
		return math.MinInt32
	}
	return int32(value)
}

func activityRunError(name string, err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return viber.FailedPreconditionf("no %s is active", name)
	}
	return err
}

func activityRefreshByID(info *papi.RefreshInfo, id string) *papi.Refresh {
	for _, refresh := range info.GetRefreshes() {
		if refresh.GetId() == id {
			return refresh
		}
	}
	return nil
}
