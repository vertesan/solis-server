package viber

import (
	"math"

	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type rewardResultKey struct {
	resourceType penum.ResourceType
	resourceID   string
}

// AggregateRewardResults coalesces repeated non-card resource results while
// preserving first-seen response order. Card results are intentionally kept
// separate because duplicate card grants carry compensation metadata whose
// meaning is tied to each individual card grant.
func AggregateRewardResults(results []*ptransaction.RewardResult) ([]*ptransaction.RewardResult, error) {
	aggregated := make([]*ptransaction.RewardResult, 0, len(results))
	byResource := make(map[rewardResultKey]*ptransaction.RewardResult, len(results))
	for _, result := range results {
		if result == nil {
			return nil, status.Error(codes.Internal, "reward result is missing")
		}
		if result.GetResourceType() == penum.ResourceType_ResourceType_Card {
			aggregated = append(aggregated, result)
			continue
		}
		key := rewardResultKey{resourceType: result.GetResourceType(), resourceID: result.GetResourceId()}
		current, exists := byResource[key]
		if !exists {
			current = proto.Clone(result).(*ptransaction.RewardResult)
			byResource[key] = current
			aggregated = append(aggregated, current)
			continue
		}
		if result.GetAmountI64() > 0 && current.GetAmountI64() > math.MaxInt64-result.GetAmountI64() {
			return nil, status.Error(codes.FailedPrecondition, "aggregated reward amount overflows")
		}
		current.AfterAmount = result.GetAfterAmount()
		current.AmountI64 += result.GetAmountI64()
		current.IsNew = current.GetIsNew() || result.GetIsNew()
		current.IsTruncate = current.GetIsTruncate() || result.GetIsTruncate()
		current.IsGift = current.GetIsGift() || result.GetIsGift()
		current.DuplicateRewardResults = append(current.DuplicateRewardResults, result.GetDuplicateRewardResults()...)
		current.AdditionalGifts = append(current.AdditionalGifts, result.GetAdditionalGifts()...)
		if result.GetIncreasedAfterEvolutionLevel() > current.GetIncreasedAfterEvolutionLevel() {
			current.IncreasedAfterEvolutionLevel = result.GetIncreasedAfterEvolutionLevel()
		}
	}
	return aggregated, nil
}
