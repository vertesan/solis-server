package activity

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/salary"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ActivityServer) FinishFanEvent(ctx context.Context, _ *papi.Empty) (*papi.ActivityFinishFanEventResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ActivityFinishFanEventResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		progress, err := opuser.FindUserFanEventRun(ctx, suid)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.FailedPrecondition, "no fan event is active")
			}
			return nil, err
		}
		now := time.Now().UnixMilli()
		state, err := viber.FindFanEventRunState(ctx, suid, progress)
		if err != nil {
			return nil, err
		}
		if _, err := viber.ReconcileFanEventProgress(ctx, suid, progress, state, now); err != nil {
			return nil, err
		}
		for _, character := range progress.GetCharacters() {
			if character.GetMaxStep() <= 0 || character.GetCompletedStep() != character.GetMaxStep() {
				return nil, status.Error(codes.FailedPrecondition, "fan event is not complete")
			}
		}
		info, err := opuser.FindUserFanEventInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		fanEvent := findUserFanEvent(info, progress.GetActivityFanEventId())
		if fanEvent == nil {
			return nil, status.Error(codes.FailedPrecondition, "active fan event catalog entry is missing")
		}
		score := fanEventScore(progress)
		rankType, rankPlus := fanEventRank(progress.GetRankPatterns(), score)
		updated := &papi.UpdatedData{}
		rewards, err := resolveFanEventRewards(
			ctx, progress, fanEvent, rankType, state.RewardCoefficientPermyriads,
		)
		if err != nil {
			return nil, err
		}
		for _, reward := range rewards {
			if err := grantActivityReward(ctx, suid, reward, updated); err != nil {
				return nil, err
			}
		}
		balanceProfile, err := findFanEventBalanceProfile(fanEvent.GetId(), fanEvent.GetLevel())
		if err != nil {
			return nil, err
		}
		coreFanCoefficient, err := fanEventCoreFanConversionMillionths(
			balanceProfile, fanEventRankKey(rankType),
		)
		if err != nil {
			return nil, err
		}
		selected := make([]*papi.ActivityCharacterInfo, 0, len(progress.GetCharacters()))
		for _, activityCharacter := range progress.GetCharacters() {
			character, err := opuser.FindUserCharacter(ctx, suid, activityCharacter.GetCharacterId())
			if err != nil {
				return nil, err
			}
			// ActivityStamina in the run is a post-image during normal saves. Derive
			// it from completed steps as well so a background-completed run cannot
			// retain the start-time stamina value.
			remainder := activityCharacter.GetActivityStamina() % progress.GetStepConsumptionStamina()
			character.ActivityStamina = max(
				int32(0),
				(activityCharacter.GetMaxStep()-activityCharacter.GetCompletedStep())*progress.GetStepConsumptionStamina()+remainder,
			)
			character.StaminaUpdatedTime = now
			character.InActivity = false
			characterActivityExp := int64(activityCharacter.GetCompletedStep())
			if balanceProfile != nil {
				characterActivityExp = balanceProfile.CharacterActivityExp
			}
			if characterActivityExp > 0 && character.GetActivityExp() > math.MaxInt64-characterActivityExp {
				return nil, status.Error(codes.FailedPrecondition, "character activity exp overflows")
			}
			character.ActivityExp += characterActivityExp
			if err := advanceCharacterActivityLevel(ctx, character); err != nil {
				return nil, err
			}
			if err := applyFanEventFanConversion(character, coreFanCoefficient); err != nil {
				return nil, err
			}
			if err := recalculateAudienceCandidateAmount(ctx, character); err != nil {
				return nil, err
			}
			if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
				return nil, err
			}
			updated.Characters = append(updated.Characters, character)
			selected = append(selected, &papi.ActivityCharacterInfo{
				CharacterId: activityCharacter.GetCharacterId(),
				CostumeId:   activityCharacter.GetCostumeId(),
				HairId:      activityCharacter.GetHairId(),
			})
		}
		before := &papi.ActivityLevelInfo{
			Exp:             fanEvent.GetExp(),
			Level:           fanEvent.GetLevel(),
			NextLevelLocked: fanEventNextLevelLocked(fanEvent, fanEvent.GetExp()),
		}
		expGain := int32(min(
			int64(math.MaxInt32),
			int64(totalFanEventSteps(progress))*3,
		))
		fanEvent.Exp = int32(min(int64(math.MaxInt32), int64(fanEvent.GetExp())+int64(expGain)))
		if err := replaceUserFanEvent(info, fanEvent); err != nil {
			return nil, err
		}
		info.ActiveFanEvent = nil
		if err := opuser.UpsertUserFanEventInfo(ctx, suid, info); err != nil {
			return nil, err
		}
		total, err := opuser.FindUserTotalCount(ctx, suid)
		if err != nil {
			return nil, err
		}
		total.ActivityFanEventCount++
		if err := opuser.UpsertUserTotalCount(ctx, suid, total); err != nil {
			return nil, err
		}
		updated.TotalCount = total
		best, err := opuser.FindUserActivityFanEventProgress(ctx, suid)
		if err != nil {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return nil, err
			}
			best = &ptransaction.UserActivityFanEventProgress{}
		}
		if rankType > best.GetBestScoreRankType() || (rankType == best.GetBestScoreRankType() && int64(rankPlus) > best.GetBestScoreRankPlus()) {
			best.BestScoreRankType = rankType
			best.BestScoreRankPlus = int64(rankPlus)
		}
		if err := opuser.UpsertUserActivityFanEventProgress(ctx, suid, best); err != nil {
			return nil, err
		}
		updated.ActivityFanEventProgress = best
		if err := updateFanEventRanking(ctx, suid, progress); err != nil {
			return nil, err
		}
		_, stepMillis, err := viber.FanEventStepDurations(ctx, progress)
		if err != nil {
			return nil, err
		}
		for _, character := range progress.GetCharacters() {
			end := min(now, progress.GetStartTime()+int64(character.GetMaxStep())*stepMillis)
			if err := salary.RecordActivity(ctx, suid, progress.GetStartTime(), end, 1); err != nil {
				return nil, err
			}
		}
		if err := opuser.DeleteUserFanEventRun(ctx, suid); err != nil {
			return nil, err
		}
		maxLevel, err := maxUnlockedFanEventLevel(ctx, suid, fanEvent.GetId(), now, fanEvent.GetLevel())
		if err != nil {
			return nil, err
		}
		nextLevelLocked := fanEventNextLevelLocked(fanEvent, fanEvent.GetExp())
		response = &papi.ActivityFinishFanEventResponse{
			ActivityPoint: int32(min(int64(math.MaxInt32), score)),
			RankType:      rankType,
			Plus:          rankPlus,
			Rewards:       rewards,
			RankPatterns:  progress.GetRankPatterns(),
			ActivityLevelProgress: &papi.ActivityLevelProgress{
				MaxActivityLevel: maxLevel,
				Before:           before,
				After:            &papi.ActivityLevelInfo{Exp: fanEvent.GetExp(), Level: fanEvent.GetLevel(), NextLevelLocked: nextLevelLocked},
			},
			Characters:          selected,
			CampaignEffects:     []*papi.ActivityCampaignEffect{},
			CampaignDropRewards: []*ptransaction.Reward{},
			CommonResponse:      viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func maxUnlockedFanEventLevel(ctx context.Context, suid, fanEventID string, now int64, fallback int32) (int32, error) {
	masters, err := opmaster.FindAllActivityFanEvents(ctx)
	if err != nil {
		return 0, err
	}
	evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now)
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

func fanEventCoreFanConversionMillionths(profile *FanEventBalanceProfile, rankKey string) (int32, error) {
	if _, err := findFanEventBalanceProfile("", 0); err != nil {
		return 0, err
	}
	if profile != nil {
		if coefficient := profile.CoreFanConversionMillionths[rankKey]; coefficient > 0 {
			return coefficient, nil
		}
	}
	coefficient := loadedFanEventBalance.CoreFanConversionMillionths[rankKey]
	if coefficient <= 0 {
		return 0, fmt.Errorf("fan event core-fan rank %q is unavailable", rankKey)
	}
	return coefficient, nil
}

func loadFanEventRewardCatalog() (*FanEventRewardCatalog, error) {
	if _, err := findFanEventBalanceProfile("", 0); err != nil {
		return nil, err
	}
	return &loadedFanEventBalance.Reward, nil
}

func findFanEventRewardProfile(id string, level int32) (*FanEventRewardProfile, error) {
	if _, err := findFanEventBalanceProfile("", 0); err != nil {
		return nil, err
	}
	for index := range loadedFanEventBalance.RewardProfiles {
		profile := &loadedFanEventBalance.RewardProfiles[index]
		if profile.ID == id && profile.Level == level {
			return profile, nil
		}
	}
	return nil, nil
}

const fanEventMaxAreaTicketBaseAmount = int64(113)

// resolveFanEventRewards turns the display-oriented fan-event Reward master
// into the resources actually earned by one completed run. The official
// catalog uses zero amounts for backend-calculated channels and adds recipe
// category examples only to the "-view" record; neither is a literal grant.
func resolveFanEventRewards(
	ctx context.Context,
	progress *papi.FanEventProgress,
	fanEvent *papi.FanEvent,
	rankType penum.ResultRankType,
	rewardCoefficientPermyriads int32,
) ([]*ptransaction.Reward, error) {
	if progress == nil || fanEvent == nil || fanEvent.GetFixedRewardId() == "" {
		return nil, status.Error(codes.FailedPrecondition, "fan event reward definition is missing")
	}
	if rewardCoefficientPermyriads <= 0 {
		rewardCoefficientPermyriads = viber.FanEventBaseRewardPermyriads
	}
	catalog, err := loadFanEventRewardCatalog()
	if err != nil {
		return nil, err
	}
	rankKey, rankSuffix := fanEventRewardRank(rankType)
	rankCoefficient := catalog.RankCoefficientsPermyriads[rankKey]
	if rankCoefficient <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "fan event reward rank is invalid")
	}

	actualRewardID := strings.TrimSuffix(fanEvent.GetFixedRewardId(), "-view")
	actualMaster, err := opmaster.FindReward(ctx, actualRewardID)
	if err != nil {
		return nil, err
	}
	areaTicketBase := fanEventAreaTicketBase(actualMaster.GetResources())
	dynamicProfile, err := findFanEventRewardProfile(fanEvent.GetId(), fanEvent.GetLevel())
	if err != nil {
		return nil, err
	}
	dynamic := fanEventDynamicReward(dynamicProfile, rankKey, progress.GetActivityFanEventType(), areaTicketBase)

	rewards := make([]*ptransaction.Reward, 0, len(actualMaster.GetResources())+3)
	managerExp := catalog.ManagerExp[rankKey]
	if areaTicketBase > 0 && areaTicketBase != fanEventMaxAreaTicketBaseAmount {
		managerExp = managerExp * areaTicketBase / fanEventMaxAreaTicketBaseAmount
	}
	managerExp, err = fanEventScaleRewardAmount(managerExp, viber.FanEventBaseRewardPermyriads, rewardCoefficientPermyriads)
	if err != nil {
		return nil, err
	}
	rewards, err = mergeFanEventReward(rewards, &ptransaction.Reward{
		ResourceType: penum.ResourceType_ResourceType_ManagerExp,
		Amount:       managerExp,
	})
	if err != nil {
		return nil, err
	}

	for index, resource := range actualMaster.GetResources() {
		if resource == nil || resource.GetAmount() < 0 {
			return nil, status.Errorf(codes.FailedPrecondition, "fan event reward %q is invalid", actualRewardID)
		}
		channel := index + 1
		channelRankCoefficient := int32(viber.FanEventBaseRewardPermyriads)
		if rankSuffix != "" {
			variantID := fmt.Sprintf("%s-%02d-%s", actualRewardID, channel, rankSuffix)
			if _, variantErr := opmaster.FindReward(ctx, variantID); variantErr == nil {
				channelRankCoefficient = rankCoefficient
			} else if !errors.Is(variantErr, mongo.ErrNoDocuments) {
				return nil, variantErr
			}
		}
		amount := int64(resource.GetAmount())
		if amount == 0 {
			switch resource.GetResourceId() {
			case "item-card_enhance":
				amount = dynamic.CardEnhance
			case "item-card_skill":
				amount = dynamic.CardSkill
			default:
				continue
			}
		}
		amount, err = fanEventScaleRewardAmount(amount, channelRankCoefficient, rewardCoefficientPermyriads)
		if err != nil {
			return nil, err
		}
		if amount <= 0 {
			continue
		}
		rewards, err = mergeFanEventReward(rewards, &ptransaction.Reward{
			ResourceType: resource.GetResourceType(),
			ResourceId:   resource.GetResourceId(),
			Amount:       amount,
		})
		if err != nil {
			return nil, err
		}
	}

	recipeRewards, err := drawFanEventRecipeRewards(
		ctx,
		fanEvent.GetFixedRewardId(),
		actualRewardID,
		catalog.RecipeDropChancePermyriads,
		rewardCoefficientPermyriads,
	)
	if err != nil {
		return nil, err
	}
	for _, reward := range recipeRewards {
		rewards, err = mergeFanEventReward(rewards, reward)
		if err != nil {
			return nil, err
		}
	}
	return rewards, nil
}

func fanEventRewardRank(rankType penum.ResultRankType) (key, suffix string) {
	switch rankType {
	case penum.ResultRankType_ResultRankType_C:
		return "C", "C"
	case penum.ResultRankType_ResultRankType_B:
		return "B", "B"
	case penum.ResultRankType_ResultRankType_A:
		return "A", "A"
	case penum.ResultRankType_ResultRankType_S:
		return "S", "S"
	case penum.ResultRankType_ResultRankType_Splus:
		return "Splus", "S1"
	default:
		return "D", ""
	}
}

func fanEventAreaTicketBase(resources []*pmaster.RewardResource) int64 {
	for _, resource := range resources {
		if resource.GetResourceId() == "item-gacha_ticket_piece-area-02" && resource.GetAmount() > 0 {
			return int64(resource.GetAmount())
		}
	}
	return fanEventMaxAreaTicketBaseAmount
}

func fanEventDynamicReward(
	profile *FanEventRewardProfile,
	rankKey string,
	fanEventType penum.ActivityFanEventType,
	areaTicketBase int64,
) FanEventDynamicReward {
	if profile != nil {
		if reward, ok := profile.DynamicByRank[rankKey]; ok {
			return reward
		}
		if reward, ok := profile.DynamicByRank["default"]; ok {
			return reward
		}
	}
	reward := FanEventDynamicReward{CardEnhance: 800000, CardSkill: 1100}
	switch fanEventType {
	case penum.ActivityFanEventType_ActivityFanEventType_Handshake:
		reward = FanEventDynamicReward{CardEnhance: 1000000, CardSkill: 1000}
	case penum.ActivityFanEventType_ActivityFanEventType_Sign:
		reward = FanEventDynamicReward{CardEnhance: 650000, CardSkill: 1200}
	}
	if areaTicketBase > 0 && areaTicketBase != fanEventMaxAreaTicketBaseAmount {
		reward.CardEnhance = max(int64(1), reward.CardEnhance*areaTicketBase/fanEventMaxAreaTicketBaseAmount)
		reward.CardSkill = max(int64(1), reward.CardSkill*areaTicketBase/fanEventMaxAreaTicketBaseAmount)
	}
	return reward
}

func fanEventScaleRewardAmount(amount int64, rankCoefficient, itemCoefficient int32) (int64, error) {
	if amount <= 0 || rankCoefficient <= 0 || itemCoefficient <= 0 {
		return 0, status.Error(codes.FailedPrecondition, "fan event reward amount is invalid")
	}
	coefficient := int64(rankCoefficient) * int64(itemCoefficient)
	if amount > math.MaxInt64/coefficient {
		return 0, status.Error(codes.FailedPrecondition, "fan event reward amount overflows")
	}
	return amount * coefficient / int64(viber.FanEventBaseRewardPermyriads) / int64(viber.FanEventBaseRewardPermyriads), nil
}

type fanEventRecipeCategory struct {
	level int32
	group string
}

func drawFanEventRecipeRewards(
	ctx context.Context,
	displayRewardID, actualRewardID string,
	dropChancePermyriads, rewardCoefficientPermyriads int32,
) ([]*ptransaction.Reward, error) {
	if !strings.HasSuffix(displayRewardID, "-view") {
		displayRewardID = actualRewardID + "-view"
	}
	display, err := opmaster.FindReward(ctx, displayRewardID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	categories := make(map[fanEventRecipeCategory]struct{})
	for _, resource := range display.GetResources() {
		if resource.GetResourceType() != penum.ResourceType_ResourceType_PhotoRecipe || resource.GetResourceId() == "" {
			continue
		}
		template, err := opmaster.FindPhotoRecipe(ctx, resource.GetResourceId())
		if err != nil {
			return nil, err
		}
		categories[fanEventRecipeCategory{level: template.GetLevel(), group: template.GetPhotoRetouchEnhanceGroupId()}] = struct{}{}
	}
	if len(categories) == 0 || dropChancePermyriads <= 0 {
		return nil, nil
	}
	masters, err := opmaster.FindAllPhotoRecipes(ctx)
	if err != nil {
		return nil, err
	}
	candidateSet := make(map[string]struct{})
	for _, recipe := range masters {
		if _, ok := categories[fanEventRecipeCategory{level: recipe.GetLevel(), group: recipe.GetPhotoRetouchEnhanceGroupId()}]; ok {
			candidateSet[recipe.GetId()] = struct{}{}
		}
	}
	candidates := make([]string, 0, len(candidateSet))
	for id := range candidateSet {
		candidates = append(candidates, id)
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "fan event recipe pool is empty")
	}
	attempts := int(rewardCoefficientPermyriads / viber.FanEventBaseRewardPermyriads)
	if remainder := rewardCoefficientPermyriads % viber.FanEventBaseRewardPermyriads; remainder > 0 {
		roll, err := viber.CryptoRandomIndex(int(viber.FanEventBaseRewardPermyriads))
		if err != nil {
			return nil, err
		}
		if roll < int(remainder) {
			attempts++
		}
	}
	rewards := make([]*ptransaction.Reward, 0, attempts)
	for attempt := 0; attempt < attempts && len(candidates) > 0; attempt++ {
		roll, err := viber.CryptoRandomIndex(int(viber.FanEventBaseRewardPermyriads))
		if err != nil {
			return nil, err
		}
		if roll >= int(dropChancePermyriads) {
			continue
		}
		index, err := viber.CryptoRandomIndex(len(candidates))
		if err != nil {
			return nil, err
		}
		rewards = append(rewards, &ptransaction.Reward{
			ResourceType: penum.ResourceType_ResourceType_PhotoRecipe,
			ResourceId:   candidates[index],
			Amount:       1,
		})
		candidates = append(candidates[:index], candidates[index+1:]...)
	}
	return rewards, nil
}

func mergeFanEventReward(rewards []*ptransaction.Reward, addition *ptransaction.Reward) ([]*ptransaction.Reward, error) {
	for _, reward := range rewards {
		if reward.GetResourceType() != addition.GetResourceType() || reward.GetResourceId() != addition.GetResourceId() {
			continue
		}
		if reward.GetAmount() > math.MaxInt64-addition.GetAmount() {
			return nil, status.Error(codes.FailedPrecondition, "fan event reward amount overflows")
		}
		reward.Amount += addition.GetAmount()
		return rewards, nil
	}
	return append(rewards, addition), nil
}

func replaceUserFanEvent(info *papi.FanEventInfo, fanEvent *papi.FanEvent) error {
	for index, current := range info.GetFanEvents() {
		if current.GetId() == fanEvent.GetId() {
			info.FanEvents[index] = fanEvent
			return nil
		}
	}
	return status.Error(codes.FailedPrecondition, "fan event catalog entry is missing")
}

func totalFanEventSteps(progress *papi.FanEventProgress) int32 {
	var total int64
	for _, character := range progress.GetCharacters() {
		total += int64(character.GetCompletedStep())
	}
	return int32(min(int64(math.MaxInt32), total))
}

func applyFanEventFanConversion(
	character *ptransaction.UserCharacter,
	coefficientMillionths int32,
) error {
	if character == nil || coefficientMillionths <= 0 {
		return nil
	}
	coefficient := int64(coefficientMillionths)
	// Core-fan growth is a maturation chain, not newly-created fans: a share
	// of light fans become middle fans while the same share of the existing
	// middle fans become heavy fans. Total fan count therefore stays constant.
	lightToMiddle := (int64(character.GetLightFanAmount())*coefficient + 500000) / 1000000
	middleToHeavy := (int64(character.GetMiddleFanAmount())*coefficient + 500000) / 1000000
	lightToMiddle = min(lightToMiddle, int64(character.GetLightFanAmount()))
	middleToHeavy = min(middleToHeavy, int64(character.GetMiddleFanAmount()))
	light := int64(character.GetLightFanAmount()) - lightToMiddle
	middle := int64(character.GetMiddleFanAmount()) + lightToMiddle - middleToHeavy
	heavy := int64(character.GetHeavyFanAmount()) + middleToHeavy
	if light < 0 || middle < 0 || heavy < 0 || light > math.MaxInt32 || middle > math.MaxInt32 || heavy > math.MaxInt32 {
		return status.Error(codes.FailedPrecondition, "core-fan conversion overflows")
	}
	character.LightFanAmount = int32(light)
	character.MiddleFanAmount = int32(middle)
	character.HeavyFanAmount = int32(heavy)
	return nil
}

func fanEventRankKey(rank penum.ResultRankType) string {
	switch rank {
	case penum.ResultRankType_ResultRankType_C:
		return "C"
	case penum.ResultRankType_ResultRankType_B:
		return "B"
	case penum.ResultRankType_ResultRankType_A:
		return "A"
	case penum.ResultRankType_ResultRankType_S:
		return "S"
	case penum.ResultRankType_ResultRankType_Splus:
		return "Splus"
	default:
		return "D"
	}
}

func fanEventNextLevelLocked(fanEvent *papi.FanEvent, exp int32) bool {
	return fanEvent != nil && fanEvent.GetNextLevelRequiredExp() > 0 &&
		exp >= fanEvent.GetNextLevelRequiredExp() && !fanEvent.GetIsNextLevelUnlocked()
}

func advanceCharacterActivityLevel(ctx context.Context, character *ptransaction.UserCharacter) error {
	for {
		next, err := opmaster.FindCharacterActivityLevel(ctx, int64(character.GetActivityLevel()+1))
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil
			}
			return err
		}
		if character.GetActivityExp() < int64(next.GetRequiredExp()) {
			return nil
		}
		character.ActivityLevel = next.GetLevel()
	}
}
