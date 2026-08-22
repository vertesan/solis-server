package viber

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const fanEventMaxAreaTicketBaseAmount = int64(113)

// ResolveFanEventRewards turns the display-oriented fan-event Reward master
// into the resources actually earned by one completed run. The official
// catalog uses zero amounts for backend-calculated channels and adds recipe
// category examples only to the "-view" record; neither is a literal grant.
func ResolveFanEventRewards(
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
		rewardCoefficientPermyriads = FanEventBaseRewardPermyriads
	}
	catalog, err := LoadFanEventRewardCatalog()
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
	dynamicProfile, err := FindFanEventRewardProfile(fanEvent.GetId(), fanEvent.GetLevel())
	if err != nil {
		return nil, err
	}
	dynamic := fanEventDynamicReward(dynamicProfile, rankKey, progress.GetActivityFanEventType(), areaTicketBase)

	rewards := make([]*ptransaction.Reward, 0, len(actualMaster.GetResources())+3)
	managerExp := catalog.ManagerExp[rankKey]
	if areaTicketBase > 0 && areaTicketBase != fanEventMaxAreaTicketBaseAmount {
		managerExp = managerExp * areaTicketBase / fanEventMaxAreaTicketBaseAmount
	}
	managerExp, err = fanEventScaleRewardAmount(managerExp, FanEventBaseRewardPermyriads, rewardCoefficientPermyriads)
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
		channelRankCoefficient := int32(FanEventBaseRewardPermyriads)
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
	return amount * coefficient / int64(FanEventBaseRewardPermyriads) / int64(FanEventBaseRewardPermyriads), nil
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
	attempts := int(rewardCoefficientPermyriads / FanEventBaseRewardPermyriads)
	if remainder := rewardCoefficientPermyriads % FanEventBaseRewardPermyriads; remainder > 0 {
		roll, err := CryptoRandomIndex(int(FanEventBaseRewardPermyriads))
		if err != nil {
			return nil, err
		}
		if roll < int(remainder) {
			attempts++
		}
	}
	rewards := make([]*ptransaction.Reward, 0, attempts)
	for attempt := 0; attempt < attempts && len(candidates) > 0; attempt++ {
		roll, err := CryptoRandomIndex(int(FanEventBaseRewardPermyriads))
		if err != nil {
			return nil, err
		}
		if roll >= int(dropChancePermyriads) {
			continue
		}
		index, err := CryptoRandomIndex(len(candidates))
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
