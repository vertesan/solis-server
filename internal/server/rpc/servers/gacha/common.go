package gacha

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func guaranteedGachaRarityPool(r *viber.GachaRules, id string) *viber.GachaRarityPool {
	for i := range r.GuaranteedRarityGachas {
		if r.GuaranteedRarityGachas[i].ID == id {
			return &r.GuaranteedRarityGachas[i]
		}
	}
	return nil
}

func currentGachaInfo(
	ctx context.Context,
	suid string,
	catalog *viber.GachaMasterCatalog,
	masterGacha *pmaster.Gacha,
	now int64,
) (*papi.GachaInfo, error) {
	if !viber.IsGachaEnabled(masterGacha) {
		return nil, status.Error(codes.PermissionDenied, "gacha is disabled")
	}
	previous, err := opuser.FindUserGachaInfo(ctx, suid, masterGacha.GetId())
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	rules, err := viber.LoadGachaRules()
	if err != nil {
		return nil, err
	}
	evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now)
	shown, err := evaluator.Satisfied(masterGacha.GetViewConditionId())
	if err != nil {
		return nil, err
	}
	if !shown {
		return nil, status.Error(codes.PermissionDenied, "gacha is not currently visible")
	}
	return viber.RenderCurrentGachaInfo(ctx, evaluator, catalog, rules, masterGacha, previous, nil, now)
}

func gachaTermEnd(ctx context.Context, conditionID string) int64 {
	_, end := gachaTerm(ctx, conditionID)
	return end
}

func gachaTerm(ctx context.Context, conditionID string) (int64, int64) {
	if conditionID == "" {
		return 0, 0
	}
	condition, err := opmaster.FindCondition(ctx, conditionID)
	if err != nil {
		return 0, 0
	}
	var start, end int64
	for _, setting := range condition.GetSettings() {
		term := setting.GetTerm()
		if term == nil {
			continue
		}
		if term.GetNowAfter() > 0 && (start == 0 || term.GetNowAfter() < start) {
			start = term.GetNowAfter()
		}
		if term.GetNowBefore() > end {
			end = term.GetNowBefore()
		}
	}
	return start, end
}

const (
	gachaHistoryLimit       = 300
	premiumFreeTicketAmount = 10
	gachaProbabilityTotal   = 100000
)

func isNormalCardGacha(masterGacha *pmaster.Gacha) bool {
	// Guaranteed-rarity tickets also use GachaType_Normal. Only the ordinary
	// diamond gacha (including its ticket button) uses the all-card pool.
	return masterGacha.GetGachaType() == penum.GachaType_GachaType_Normal &&
		strings.HasPrefix(masterGacha.GetId(), "gacha-normal-")
}

type normalGachaTier struct {
	rarity  int32
	weight  int64
	rewards []*pmaster.RewardResource
}

func buildNormalGachaTiers(ctx context.Context, pool []*pmaster.RewardResource) ([]normalGachaTier, error) {
	if len(pool) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "normal gacha reward pool is empty")
	}
	rules, err := viber.LoadGachaRules()
	if err != nil {
		return nil, err
	}
	cards, err := opmaster.FindAllCards(ctx)
	if err != nil {
		return nil, err
	}
	cardRarities := make(map[string]int32, len(cards))
	for _, card := range cards {
		cardRarities[card.GetId()] = card.GetInitialRarity()
	}
	byRarity := make(map[int32][]*pmaster.RewardResource)
	for _, reward := range pool {
		if reward.GetResourceType() != penum.ResourceType_ResourceType_Card || reward.GetAmount() != 1 {
			return nil, status.Error(codes.FailedPrecondition, "normal gacha pool must contain single-card rewards")
		}
		rarity := cardRarities[reward.GetResourceId()]
		if rules.StandardCardRarityProbabilities[rarity] <= 0 {
			return nil, status.Errorf(codes.FailedPrecondition, "card %q has unsupported gacha rarity %d", reward.GetResourceId(), rarity)
		}
		byRarity[rarity] = append(byRarity[rarity], reward)
	}
	tiers := make([]normalGachaTier, 0, len(byRarity))
	var total int64
	for rarity, rewards := range byRarity {
		weight := rules.StandardCardRarityProbabilities[rarity]
		if weight > math.MaxInt64-total {
			return nil, status.Error(codes.FailedPrecondition, "normal gacha rarity weights overflow")
		}
		total += weight
		tiers = append(tiers, normalGachaTier{rarity: rarity, weight: weight, rewards: rewards})
	}
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].rarity > tiers[j].rarity })
	return tiers, nil
}

func buildGachaPool(
	ctx context.Context,
	masterGacha *pmaster.Gacha,
	state *papi.GachaInfo,
	now int64,
) ([]*pmaster.RewardResource, error) {
	var pool []*pmaster.RewardResource
	switch masterGacha.GetGachaType() {
	case penum.GachaType_GachaType_Item:
		pool = transactionRewardsToMaster(state.GetItemGachaRewards())
		if len(pool) == 0 {
			return nil, status.Error(codes.FailedPrecondition, "item gacha has no configured reward pool")
		}
	case penum.GachaType_GachaType_ShowcaseToy:
		pool = transactionRewardsToMaster(state.GetShowcaseToyGachaRewards())
		if len(pool) == 0 {
			return nil, status.Error(codes.FailedPrecondition, "showcase-toy gacha has no configured reward pool")
		}
	case penum.GachaType_GachaType_Premium:
		weighted, err := viber.BuildPremiumGachaPool(ctx, masterGacha)
		if err != nil {
			return nil, err
		}
		pool = make([]*pmaster.RewardResource, 0, len(weighted))
		for _, entry := range weighted {
			pool = append(pool, proto.Clone(entry.Reward).(*pmaster.RewardResource))
		}
	}
	if len(pool) > 0 {
		return pool, nil
	}
	rules, err := viber.LoadGachaRules()
	if err != nil {
		return nil, err
	}
	if isNormalCardGacha(masterGacha) {
		cards, err := opmaster.FindAllCards(ctx)
		if err != nil {
			return nil, err
		}
		// Pickup IDs describe the banner, not the server's draw pool. Include
		// every card in a configured rarity tier, including limited cards.
		pool = make([]*pmaster.RewardResource, 0, len(cards))
		for _, card := range cards {
			if rules.StandardCardRarityProbabilities[card.GetInitialRarity()] <= 0 {
				continue
			}
			pool = append(pool, &pmaster.RewardResource{
				ResourceType: penum.ResourceType_ResourceType_Card,
				ResourceId:   card.GetId(),
				Amount:       1,
			})
		}
		return pool, nil
	}
	if rarityPool := guaranteedGachaRarityPool(rules, masterGacha.GetId()); rarityPool != nil {
		if rarityPool.UseMasterCardRewards {
			return masterGachaCardRewardPool(ctx, masterGacha, rarityPool.MinimumRarity)
		}
		return fallbackCardPool(ctx, now, rarityPool.MinimumRarity)
	}
	return fallbackCardPool(ctx, now, 3)
}

func masterGachaCardRewardPool(ctx context.Context, masterGacha *pmaster.Gacha, minimumRarity int32) ([]*pmaster.RewardResource, error) {
	pool := make([]*pmaster.RewardResource, 0, len(masterGacha.GetCardRewards()))
	seen := make(map[string]struct{}, len(masterGacha.GetCardRewards()))
	for _, definition := range masterGacha.GetCardRewards() {
		cardID := definition.GetCardId()
		if cardID == "" {
			return nil, status.Error(codes.FailedPrecondition, "master-derived gacha pool contains an empty card ID")
		}
		if _, exists := seen[cardID]; exists {
			continue
		}
		card, err := opmaster.FindCard(ctx, cardID)
		if err != nil {
			return nil, err
		}
		if card.GetInitialRarity() < minimumRarity {
			return nil, status.Errorf(codes.FailedPrecondition, "master-derived gacha card %q is below guaranteed rarity %d", cardID, minimumRarity)
		}
		seen[cardID] = struct{}{}
		pool = append(pool, &pmaster.RewardResource{
			ResourceType: penum.ResourceType_ResourceType_Card,
			ResourceId:   cardID,
			Amount:       1,
		})
	}
	if len(pool) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "master-derived guaranteed-rarity gacha pool is empty")
	}
	return pool, nil
}

func fallbackCardPool(ctx context.Context, now int64, minimumRarity int32) ([]*pmaster.RewardResource, error) {
	cards, err := opmaster.FindAllCards(ctx)
	if err != nil {
		return nil, err
	}
	pool := make([]*pmaster.RewardResource, 0, len(cards))
	for _, card := range cards {
		if card.GetInitialRarity() < minimumRarity || card.GetReleaseDate() > now {
			continue
		}
		pool = append(pool, &pmaster.RewardResource{
			ResourceType: penum.ResourceType_ResourceType_Card,
			ResourceId:   card.GetId(),
			Amount:       1,
		})
	}
	return pool, nil
}

func transactionRewardsToMaster(rewards []*ptransaction.Reward) []*pmaster.RewardResource {
	result := make([]*pmaster.RewardResource, 0, len(rewards))
	for _, reward := range rewards {
		if reward.GetAmount() <= 0 || reward.GetAmount() > math.MaxInt32 {
			continue
		}
		result = append(result, &pmaster.RewardResource{
			ResourceType: reward.GetResourceType(),
			ResourceId:   reward.GetResourceId(),
			Amount:       int32(reward.GetAmount()),
		})
	}
	return result
}

func gachaRewardWeights(
	ctx context.Context,
	masterGacha *pmaster.Gacha,
	selectedCardIDs []string,
	pool []*pmaster.RewardResource,
) ([]int64, error) {
	if len(pool) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "gacha reward pool is empty")
	}
	if masterGacha.GetGachaType() == penum.GachaType_GachaType_Premium {
		weighted, err := viber.BuildPremiumGachaPool(ctx, masterGacha)
		if err != nil {
			return nil, err
		}
		if len(weighted) != len(pool) {
			return nil, status.Error(codes.FailedPrecondition, "premium gacha pool does not match its probability catalog")
		}
		weights := make([]int64, len(pool))
		for i, entry := range weighted {
			if !sameGachaReward(entry.Reward, pool[i]) {
				return nil, status.Error(codes.FailedPrecondition, "premium gacha pool order does not match its probability catalog")
			}
			weights[i] = entry.Probability
		}
		return weights, nil
	}
	if masterGacha.GetGachaType() == penum.GachaType_GachaType_Item ||
		masterGacha.GetGachaType() == penum.GachaType_GachaType_ShowcaseToy {
		return uniformGachaWeights(len(pool)), nil
	}

	rules, err := viber.LoadGachaRules()
	if err != nil {
		return nil, err
	}
	cards, err := opmaster.FindAllCards(ctx)
	if err != nil {
		return nil, err
	}
	cardRarity := make(map[string]int32, len(cards))
	for _, card := range cards {
		cardRarity[card.GetId()] = card.GetInitialRarity()
	}
	buckets := make(map[int32][]int)
	for i, reward := range pool {
		if reward.GetResourceType() != penum.ResourceType_ResourceType_Card {
			return nil, status.Error(codes.FailedPrecondition, "standard gacha pool contains a non-card reward")
		}
		rarity := cardRarity[reward.GetResourceId()]
		if rules.StandardCardRarityProbabilities[rarity] <= 0 {
			return nil, status.Errorf(codes.FailedPrecondition, "card %q has unsupported gacha rarity %d", reward.GetResourceId(), rarity)
		}
		buckets[rarity] = append(buckets[rarity], i)
	}
	rarities := make([]int, 0, len(buckets))
	var configuredTotal int64
	for rarity := range buckets {
		rarities = append(rarities, int(rarity))
		configuredTotal += rules.StandardCardRarityProbabilities[rarity]
	}
	sort.Ints(rarities)
	weights := make([]int64, len(pool))
	remainingTotal := int64(gachaProbabilityTotal)
	selected := viber.StringSet(selectedCardIDs)
	pickups := viber.StringSet(masterGacha.GetPickupCardIds())
	for rarityIndex, rarityValue := range rarities {
		rarity := int32(rarityValue)
		bucketTotal := int64(gachaProbabilityTotal) * rules.StandardCardRarityProbabilities[rarity] / configuredTotal
		if rarityIndex == len(rarities)-1 {
			bucketTotal = remainingTotal
		}
		remainingTotal -= bucketTotal
		indices := buckets[rarity]
		ratioUpIndices := make([]int, 0)
		regularIndices := make([]int, 0, len(indices))
		for _, poolIndex := range indices {
			resourceID := pool[poolIndex].GetResourceId()
			_, pickup := pickups[resourceID]
			_, chosen := selected[resourceID]
			if pickup || chosen {
				ratioUpIndices = append(ratioUpIndices, poolIndex)
			} else {
				regularIndices = append(regularIndices, poolIndex)
			}
		}
		ratioUpTotal := rules.RatioUpCardProbability * int64(len(ratioUpIndices))
		if len(regularIndices) == 0 || ratioUpTotal >= bucketTotal {
			assignGachaWeightEvenly(weights, ratioUpIndices, bucketTotal)
			continue
		}
		for _, poolIndex := range ratioUpIndices {
			weights[poolIndex] = rules.RatioUpCardProbability
		}
		assignGachaWeightEvenly(weights, regularIndices, bucketTotal-ratioUpTotal)
	}
	return weights, nil
}

func assignGachaWeightEvenly(weights []int64, indices []int, total int64) {
	if len(indices) == 0 {
		return
	}
	remaining := total
	for i, poolIndex := range indices {
		weight := total / int64(len(indices))
		if i == len(indices)-1 {
			weight = remaining
		}
		remaining -= weight
		weights[poolIndex] = weight
	}
}

func uniformGachaWeights(count int) []int64 {
	weights := make([]int64, count)
	remaining := int64(gachaProbabilityTotal)
	for i := range weights {
		weight := int64(gachaProbabilityTotal) / int64(count)
		if i == count-1 {
			weight = remaining
		}
		remaining -= weight
		weights[i] = weight
	}
	return weights
}

func sameGachaReward(left, right *pmaster.RewardResource) bool {
	return left.GetResourceType() == right.GetResourceType() &&
		left.GetResourceId() == right.GetResourceId() &&
		left.GetAmount() == right.GetAmount()
}
