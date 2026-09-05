package gacha

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *GachaServer) Probability(ctx context.Context, req *papi.GachaProbabilityRequest) (*papi.GachaProbabilityResponse, error) {
	if req.GetGachaId() == "" {
		return nil, status.Error(codes.InvalidArgument, "gachaId is required")
	}
	suid := viber.GetSuid(ctx).String()
	catalog, err := viber.LoadGachaMasterCatalog(ctx)
	if err != nil {
		return nil, err
	}
	masterGacha := catalog.Gachas[req.GetGachaId()]
	if masterGacha == nil {
		return nil, status.Error(codes.NotFound, "gacha not found")
	}
	now := time.Now().UnixMilli()
	state, err := currentGachaInfo(ctx, suid, catalog, masterGacha, now)
	if err != nil {
		return nil, err
	}
	if !state.GetUnlocked() {
		return nil, status.Error(codes.PermissionDenied, "gacha is locked")
	}
	pool, err := buildGachaPool(ctx, masterGacha, state, now)
	if err != nil {
		return nil, err
	}
	probabilities, rarityProbabilities, err := calculateGachaProbabilities(ctx, masterGacha, state, pool)
	if err != nil {
		return nil, err
	}
	return &papi.GachaProbabilityResponse{
		RarityProbabilityInfos: rarityProbabilities,
		ProbabilityInfos:       probabilities,
		CommonResponse:         viber.NewCommonResponse(nil),
	}, nil
}

func calculateGachaProbabilities(
	ctx context.Context,
	masterGacha *pmaster.Gacha,
	state *papi.GachaInfo,
	pool []*pmaster.RewardResource,
) ([]*papi.ProbabilityInfo, []*papi.RarityProbabilityInfo, error) {
	if len(pool) == 0 {
		return []*papi.ProbabilityInfo{}, []*papi.RarityProbabilityInfo{}, nil
	}
	selected := viber.StringSet(state.GetSelectedCardIds())
	pickups := viber.StringSet(masterGacha.GetPickupCardIds())
	weights, err := gachaRewardWeights(ctx, masterGacha, state.GetSelectedCardIds(), pool)
	if err != nil {
		return nil, nil, err
	}
	var totalWeight int64
	for _, weight := range weights {
		totalWeight += weight
	}

	probabilities := make([]*papi.ProbabilityInfo, 0, len(pool))
	remaining := int64(gachaProbabilityTotal)
	isPremium := masterGacha.GetGachaType() == penum.GachaType_GachaType_Premium
	for i, reward := range pool {
		probability := weights[i]
		if !isPremium {
			probability = int64(gachaProbabilityTotal) * weights[i] / totalWeight
			if i == len(pool)-1 {
				probability = remaining
			}
			remaining -= probability
		}
		if probability <= 0 || probability > math.MaxInt32 {
			return nil, nil, status.Error(codes.FailedPrecondition, "gacha reward probability is invalid")
		}
		_, isPickup := pickups[reward.GetResourceId()]
		_, isSelected := selected[reward.GetResourceId()]
		probabilities = append(probabilities, &papi.ProbabilityInfo{
			ResourceType: reward.GetResourceType(),
			ResourceId:   reward.GetResourceId(),
			Amount:       int64(reward.GetAmount()),
			Probability:  int32(probability),
			IsRatioUp:    isPickup || isSelected,
		})
	}

	if isPremium {
		return probabilities, []*papi.RarityProbabilityInfo{}, nil
	}
	cards, err := opmaster.FindAllCards(ctx)
	if err != nil {
		return nil, nil, err
	}
	cardRarity := make(map[string]int32, len(cards))
	for _, card := range cards {
		cardRarity[card.GetId()] = card.GetInitialRarity()
	}
	byRarity := make(map[int32][]*papi.ProbabilityInfo)
	for _, probability := range probabilities {
		if probability.GetResourceType() != penum.ResourceType_ResourceType_Card {
			continue
		}
		rarity := cardRarity[probability.GetResourceId()]
		if rarity > 0 {
			byRarity[rarity] = append(byRarity[rarity], probability)
		}
	}
	rarities := make([]int, 0, len(byRarity))
	for rarity := range byRarity {
		rarities = append(rarities, int(rarity))
	}
	sort.Sort(sort.Reverse(sort.IntSlice(rarities)))
	rarityProbabilities := make([]*papi.RarityProbabilityInfo, 0, len(rarities))
	for _, rarity := range rarities {
		entries := byRarity[int32(rarity)]
		var total int32
		for _, entry := range entries {
			total += entry.GetProbability()
		}
		pattern := penum.GachaRewardPatternType_GachaRewardPatternType_NotSelectCard
		if masterGacha.GetSelectPickupAmount() > 0 {
			pattern = penum.GachaRewardPatternType_GachaRewardPatternType_SelectCard
		} else if masterGacha.GetGachaType() == penum.GachaType_GachaType_Premium {
			pattern = penum.GachaRewardPatternType_GachaRewardPatternType_Premium
		}
		individual := int32(0)
		if len(entries) > 0 {
			individual = entries[0].GetProbability()
		}
		rarityProbabilities = append(rarityProbabilities, &papi.RarityProbabilityInfo{
			Rarity:                 int32(rarity),
			Probability:            total,
			ProbabilityInfos:       entries,
			GachaRewardPatternType: pattern,
			IndividualProbability:  individual,
		})
	}
	return probabilities, rarityProbabilities, nil
}
