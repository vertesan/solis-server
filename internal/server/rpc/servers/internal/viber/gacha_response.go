package viber

import (
	"math"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type gachaRewardResultKey struct {
	resourceType penum.ResourceType
	resourceID   string
}

type gachaItemKey struct {
	itemID      string
	expiredTime int64
}

// AggregateGachaRewardResults coalesces every repeated drawn/granted resource,
// including cards. Nested duplicate rewards and additional gifts are coalesced
// by the same resource identity so repeated cards retain one canonical result.
func AggregateGachaRewardResults(results []*ptransaction.RewardResult) ([]*ptransaction.RewardResult, error) {
	return aggregateGachaRewardResults(results)
}

func aggregateGachaRewardResults(results []*ptransaction.RewardResult) ([]*ptransaction.RewardResult, error) {
	aggregated := make([]*ptransaction.RewardResult, 0, len(results))
	byResource := make(map[gachaRewardResultKey]*ptransaction.RewardResult, len(results))
	for _, result := range results {
		if result == nil {
			return nil, status.Error(codes.Internal, "gacha reward result is missing")
		}
		candidate := proto.Clone(result).(*ptransaction.RewardResult)
		var err error
		candidate.DuplicateRewardResults, err = aggregateGachaRewardResults(candidate.GetDuplicateRewardResults())
		if err != nil {
			return nil, err
		}
		candidate.AdditionalGifts, err = aggregateGachaRewards(candidate.GetAdditionalGifts())
		if err != nil {
			return nil, err
		}
		key := gachaRewardResultKey{resourceType: result.GetResourceType(), resourceID: result.GetResourceId()}
		current := byResource[key]
		if current == nil {
			current = candidate
			byResource[key] = current
			aggregated = append(aggregated, current)
			continue
		}
		if candidate.GetAmountI64() > 0 && current.GetAmountI64() > math.MaxInt64-candidate.GetAmountI64() {
			return nil, status.Error(codes.FailedPrecondition, "aggregated gacha reward amount overflows")
		}
		currentIsAdditive := isAdditiveGachaRewardResult(current)
		candidateIsAdditive := isAdditiveGachaRewardResult(candidate)
		duplicateResults, err := aggregateGachaRewardResults(append(current.GetDuplicateRewardResults(), candidate.GetDuplicateRewardResults()...))
		if err != nil {
			return nil, err
		}
		additionalGifts, err := aggregateGachaRewards(append(current.GetAdditionalGifts(), candidate.GetAdditionalGifts()...))
		if err != nil {
			return nil, err
		}
		current.AmountI64 += candidate.GetAmountI64()
		if currentIsAdditive && candidateIsAdditive && current.GetBeforeAmount() <= math.MaxInt64-current.GetAmountI64() {
			// Official grouped results report an identity-local post-image even
			// when grants for other resources were interleaved in the draw.
			current.AfterAmount = current.GetBeforeAmount() + current.GetAmountI64()
		} else {
			current.AfterAmount = candidate.GetAfterAmount()
		}
		current.IsNew = current.GetIsNew() || candidate.GetIsNew()
		current.IsTruncate = current.GetIsTruncate() || candidate.GetIsTruncate()
		current.IsGift = current.GetIsGift() || candidate.GetIsGift()
		current.DuplicateRewardResults = duplicateResults
		current.AdditionalGifts = additionalGifts
		if candidate.GetIncreasedAfterEvolutionLevel() > current.GetIncreasedAfterEvolutionLevel() {
			current.IncreasedAfterEvolutionLevel = candidate.GetIncreasedAfterEvolutionLevel()
		}
	}
	return aggregated, nil
}

func isAdditiveGachaRewardResult(result *ptransaction.RewardResult) bool {
	return result.GetResourceType() != penum.ResourceType_ResourceType_Card &&
		!result.GetIsTruncate() && result.GetAmountI64() >= 0 && result.GetBeforeAmount() >= 0 &&
		result.GetBeforeAmount() <= math.MaxInt64-result.GetAmountI64() &&
		result.GetAfterAmount() == result.GetBeforeAmount()+result.GetAmountI64()
}

func aggregateGachaRewards(rewards []*ptransaction.Reward) ([]*ptransaction.Reward, error) {
	aggregated := make([]*ptransaction.Reward, 0, len(rewards))
	byResource := make(map[gachaRewardResultKey]*ptransaction.Reward, len(rewards))
	for _, reward := range rewards {
		if reward == nil {
			return nil, status.Error(codes.Internal, "additional gacha reward is missing")
		}
		key := gachaRewardResultKey{resourceType: reward.GetResourceType(), resourceID: reward.GetResourceId()}
		current := byResource[key]
		if current == nil {
			current = proto.Clone(reward).(*ptransaction.Reward)
			byResource[key] = current
			aggregated = append(aggregated, current)
			continue
		}
		if reward.GetAmount() > 0 && current.GetAmount() > math.MaxInt64-reward.GetAmount() {
			return nil, status.Error(codes.FailedPrecondition, "aggregated additional gacha reward amount overflows")
		}
		current.Amount += reward.GetAmount()
	}
	return aggregated, nil
}

func CanonicalizeGachaUpdatedData(updated *papi.UpdatedData) {
	if updated == nil {
		return
	}
	updated.Items = dedupeGachaUpdates(updated.Items, func(value *ptransaction.UserItem) gachaItemKey {
		return gachaItemKey{itemID: value.GetItemId(), expiredTime: value.GetExpiredTime()}
	})
	updated.Cards = dedupeGachaUpdates(updated.Cards, func(value *ptransaction.UserCard) string { return value.GetCardId() })
	updated.Characters = dedupeGachaUpdates(updated.Characters, func(value *ptransaction.UserCharacter) string { return value.GetCharacterId() })
	updated.Costumes = dedupeGachaUpdates(updated.Costumes, func(value *ptransaction.UserCostume) string { return value.GetCostumeId() })
	updated.Accessories = dedupeGachaUpdates(updated.Accessories, func(value *ptransaction.UserAccessory) string { return value.GetAccessoryId() })
	updated.Stories = dedupeGachaUpdates(updated.Stories, func(value *ptransaction.UserStory) string { return value.GetStoryId() })
	updated.Points = dedupeGachaUpdates(updated.Points, func(value *ptransaction.UserPoint) penum.PointType { return value.GetPointType() })
	updated.Emblems = dedupeGachaUpdates(updated.Emblems, func(value *ptransaction.UserEmblem) string { return value.GetEmblemId() })
	updated.Missions = dedupeGachaUpdates(updated.Missions, func(value *ptransaction.UserMission) string { return value.GetMissionId() })
	updated.GachaButtons = dedupeGachaUpdates(updated.GachaButtons, func(value *ptransaction.UserGachaButton) string { return value.GetGachaButtonId() })
	updated.Notifications = dedupeGachaUpdates(updated.Notifications, func(value *ptransaction.UserNotification) penum.NotificationType { return value.GetNotificationType() })
	updated.Decorations = dedupeGachaUpdates(updated.Decorations, func(value *ptransaction.UserDecoration) string { return value.GetDecorationId() })
	updated.PhotoRecipes = dedupeGachaUpdates(updated.PhotoRecipes, func(value *ptransaction.UserPhotoRecipe) string { return value.GetPhotoRecipeId() })
	updated.Hairs = dedupeGachaUpdates(updated.Hairs, func(value *ptransaction.UserHair) string { return value.GetHairId() })
	updated.ShowcaseToys = dedupeGachaUpdates(updated.ShowcaseToys, func(value *ptransaction.UserShowcaseToy) string { return value.GetShowcaseToyId() })
	updated.PhotoPoses = dedupeGachaUpdates(updated.PhotoPoses, func(value *ptransaction.UserPhotoPose) string { return value.GetPhotoPoseId() })
}

func dedupeGachaUpdates[T any, K comparable](values []*T, key func(*T) K) []*T {
	result := make([]*T, 0, len(values))
	indices := make(map[K]int, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		identity := key(value)
		if index, ok := indices[identity]; ok {
			result[index] = value
			continue
		}
		indices[identity] = len(result)
		result = append(result, value)
	}
	return result
}
