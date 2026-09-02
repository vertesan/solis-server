package viber

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	usercostume "github.com/vertesan/solis-server/internal/makino/user/costume"
	userphoto "github.com/vertesan/solis-server/internal/makino/user/photo"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrExchangeInsufficientResource = errors.New("insufficient exchange resource")

const photoRecipeCapacityGiftMessage = "フォトレシピの所持上限を超えたためギフトに送られました"

func FindMasterReward(ctx context.Context, rewardId string) (*pmaster.Reward, error) {
	return opmaster.FindReward(ctx, rewardId)
}

func ParseAutoReward(rewardId string) ([]*pmaster.RewardResource, error) {
	if !strings.HasPrefix(rewardId, "rd_auto;") {
		return nil, nil
	}
	parts := strings.Split(rewardId, ";")
	if len(parts) != 4 {
		return nil, status.Error(codes.FailedPrecondition, "malformed automatic reward")
	}
	resourceType, ok := autoRewardResourceTypes[parts[1]]
	if !ok {
		return nil, status.Errorf(codes.Unimplemented, "automatic reward type %q is unsupported", parts[1])
	}
	amount, err := strconv.ParseInt(parts[3], 10, 32)
	if err != nil || amount <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "malformed automatic reward amount")
	}
	return []*pmaster.RewardResource{{
		ResourceType: resourceType,
		ResourceId:   parts[2],
		Amount:       int32(amount),
	}}, nil
}

// ResolveRewardResources expands either an inline rd_auto reward or a master
// Reward and scales every resource by count.
func ResolveRewardResources(ctx context.Context, rewardID string, count int32) ([]*pmaster.RewardResource, error) {
	if rewardID == "" || count <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "reward and positive count are required")
	}
	resources, err := ParseAutoReward(rewardID)
	if err != nil {
		return nil, err
	}
	if resources == nil {
		reward, err := FindMasterReward(ctx, rewardID)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Errorf(codes.FailedPrecondition, "reward %q is missing from master data", rewardID)
			}
			return nil, err
		}
		resources = reward.GetResources()
	}
	resolved := make([]*pmaster.RewardResource, 0, len(resources))
	for _, resource := range resources {
		if resource == nil || resource.GetAmount() <= 0 ||
			int64(resource.GetAmount()) > math.MaxInt32/int64(count) {
			return nil, status.Error(codes.FailedPrecondition, "reward amount is invalid")
		}
		resolved = append(resolved, &pmaster.RewardResource{
			ResourceType: resource.GetResourceType(),
			ResourceId:   resource.GetResourceId(),
			Amount:       resource.GetAmount() * count,
		})
	}
	if len(resolved) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "reward has no resources")
	}
	return resolved, nil
}

var autoRewardResourceTypes = map[string]penum.ResourceType{
	"item":                                 penum.ResourceType_ResourceType_Item,
	"accessory":                            penum.ResourceType_ResourceType_Accessory,
	"costume":                              penum.ResourceType_ResourceType_Costume,
	"gold":                                 penum.ResourceType_ResourceType_Gold,
	"stone":                                penum.ResourceType_ResourceType_Stone,
	"paid_stone":                           penum.ResourceType_ResourceType_PaidStone,
	"activate_photo_shooting_story_ticket": penum.ResourceType_ResourceType_ActivatePhotoShootingStoryTicket,
	"card_rarity_exp":                      penum.ResourceType_ResourceType_CardRarityExp,
	"photo_recipe":                         penum.ResourceType_ResourceType_PhotoRecipe,
	"showcase_toy":                         penum.ResourceType_ResourceType_ShowcaseToy,
	"showcase_music":                       penum.ResourceType_ResourceType_ShowcaseMusic,
	"photo_pose":                           penum.ResourceType_ResourceType_PhotoPose,
	"photo_all_in_one":                     penum.ResourceType_ResourceType_PhotoAllInOne,
	"company_point":                        penum.ResourceType_ResourceType_CompanyPoint,
	"guild_medal":                          penum.ResourceType_ResourceType_GuildMedal,
}

func ValidateExchangeResources(costType penum.ResourceType, rewards []*pmaster.RewardResource) error {
	if !isSupportedExchangeCost(costType) {
		return status.Errorf(codes.Unimplemented, "exchange cost resource type %s is unsupported", costType)
	}
	return ValidateExchangeRewardResources(rewards)
}

// ValidateExchangeRewardResources validates a complete reward before callers
// begin a transaction's writes. GrantExchangeRewards still validates concrete
// master identities while applying each resource.
func ValidateExchangeRewardResources(rewards []*pmaster.RewardResource) error {
	for _, reward := range rewards {
		if reward == nil || reward.GetAmount() <= 0 {
			return status.Error(codes.FailedPrecondition, "reward resource is invalid")
		}
		if !isSupportedExchangeReward(reward.ResourceType) {
			return status.Errorf(codes.Unimplemented, "exchange reward resource type %s is unsupported", reward.ResourceType)
		}
	}
	return nil
}

func isSupportedExchangeCost(resourceType penum.ResourceType) bool {
	if resourceType == penum.ResourceType_ResourceType_Item ||
		resourceType == penum.ResourceType_ResourceType_Stone ||
		resourceType == penum.ResourceType_ResourceType_PaidStone {
		return true
	}
	_, ok := exchangePointType(resourceType)
	return ok
}

func isSupportedExchangeReward(resourceType penum.ResourceType) bool {
	switch resourceType {
	case penum.ResourceType_ResourceType_Item,
		penum.ResourceType_ResourceType_Accessory,
		penum.ResourceType_ResourceType_Costume,
		penum.ResourceType_ResourceType_Stone,
		penum.ResourceType_ResourceType_PaidStone,
		penum.ResourceType_ResourceType_CardRarityExp,
		penum.ResourceType_ResourceType_CardRarityExpLimitChange,
		penum.ResourceType_ResourceType_PhotoRecipe,
		penum.ResourceType_ResourceType_Emblem,
		penum.ResourceType_ResourceType_Decoration,
		penum.ResourceType_ResourceType_Hair,
		penum.ResourceType_ResourceType_CompanyPoint,
		penum.ResourceType_ResourceType_HierarchyFixedPoint,
		penum.ResourceType_ResourceType_HierarchyVariablePoint,
		penum.ResourceType_ResourceType_ShowcaseToy,
		penum.ResourceType_ResourceType_ShowcaseMusic,
		penum.ResourceType_ResourceType_PhotoPose,
		penum.ResourceType_ResourceType_PhotoAllInOne:
		return true
	default:
		_, ok := exchangePointType(resourceType)
		return ok
	}
}

func ConsumeExchangeResource(
	ctx context.Context,
	suid string,
	resourceType penum.ResourceType,
	resourceId string,
	amount int64,
	updated *papi.UpdatedData,
) error {
	if amount <= 0 {
		return status.Error(codes.FailedPrecondition, "exchange cost must be positive")
	}
	switch resourceType {
	case penum.ResourceType_ResourceType_Item:
		return consumeExchangeItems(ctx, suid, resourceId, amount, updated)
	case penum.ResourceType_ResourceType_Stone, penum.ResourceType_ResourceType_PaidStone:
		return consumeExchangeBalance(ctx, suid, resourceType, amount, updated)
	default:
		pointType, ok := exchangePointType(resourceType)
		if !ok {
			return status.Errorf(codes.Unimplemented, "exchange cost resource type %s is unsupported", resourceType)
		}
		return consumeExchangePoint(ctx, suid, pointType, amount, updated)
	}
}

func consumeExchangeItems(ctx context.Context, suid, itemId string, amount int64, updated *papi.UpdatedData) error {
	_, err := ConsumeExchangeItemStacks(ctx, suid, itemId, amount, updated)
	return err
}

// ConsumeExchangeItemStacks consumes dated item stacks nearest-expiry first
// and returns one result per mutated stack, matching the stack post-images in
// UpdatedData.Items.
func ConsumeExchangeItemStacks(
	ctx context.Context,
	suid string,
	itemId string,
	amount int64,
	updated *papi.UpdatedData,
) ([]*ptransaction.ConsumptionResult, error) {
	if amount <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "exchange cost must be positive")
	}
	items, err := opuser.FindUserItems(ctx, suid, itemId)
	if err != nil {
		return nil, err
	}
	// Mongo sorts the permanent stack (expiredTime == 0) before dated stacks.
	// Consume dated currency by nearest expiry first and preserve permanent
	// currency until every usable dated stack has been exhausted.
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i].ExpiredTime, items[j].ExpiredTime
		if left == 0 {
			return false
		}
		if right == 0 {
			return true
		}
		return left < right
	})
	now := time.Now().UnixMilli()
	var total int64
	for _, item := range items {
		if item.Amount > 0 && (item.ExpiredTime == 0 || item.ExpiredTime > now) {
			total += item.Amount
		}
	}
	if total < amount {
		return nil, ErrExchangeInsufficientResource
	}
	remaining := amount
	results := make([]*ptransaction.ConsumptionResult, 0, len(items))
	for _, item := range items {
		if remaining == 0 || item.Amount <= 0 || (item.ExpiredTime > 0 && item.ExpiredTime <= now) {
			continue
		}
		before := item.Amount
		consumed := min(item.Amount, remaining)
		item.Amount -= consumed
		remaining -= consumed
		if err := opuser.UpsertUserItem(ctx, suid, item); err != nil {
			return nil, err
		}
		updated.Items = append(updated.Items, item)
		results = append(results, &ptransaction.ConsumptionResult{
			ResourceType: penum.ResourceType_ResourceType_Item,
			ResourceId:   itemId,
			Amount:       consumed,
			BeforeAmount: before,
			AfterAmount:  item.Amount,
		})
	}
	return results, nil
}

func consumeExchangePoint(ctx context.Context, suid string, pointType penum.PointType, amount int64, updated *papi.UpdatedData) error {
	point, err := opuser.FindUserPoint(ctx, suid, pointType)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrExchangeInsufficientResource
		}
		return err
	}
	if point.Amount < amount {
		return ErrExchangeInsufficientResource
	}
	point.Amount -= amount
	if err := opuser.UpsertUserPoint(ctx, suid, point); err != nil {
		return err
	}
	updated.Points = append(updated.Points, point)
	return nil
}

func consumeExchangeBalance(ctx context.Context, suid string, resourceType penum.ResourceType, amount int64, updated *papi.UpdatedData) error {
	if amount > math.MaxInt32 {
		return ErrExchangeInsufficientResource
	}
	balance, err := opuser.FindUserBalance(ctx, suid)
	if err != nil {
		return err
	}
	if resourceType == penum.ResourceType_ResourceType_PaidStone {
		if int64(balance.PaidBalance) < amount {
			return ErrExchangeInsufficientResource
		}
		balance.PaidBalance -= int32(amount)
	} else {
		if int64(balance.FreeBalance) < amount {
			return ErrExchangeInsufficientResource
		}
		balance.FreeBalance -= int32(amount)
	}
	if err := opuser.UpsertUserBalance(ctx, suid, balance); err != nil {
		return err
	}
	updated.Balance = balance
	return nil
}

// GrantExchangeRewards applies one reward resource and returns every resource
// unlocked by it. Most resources have one result, while a newly-owned costume
// also grants its master-data default hair.
func GrantExchangeRewards(ctx context.Context, suid string, reward *pmaster.RewardResource, updated *papi.UpdatedData) ([]*ptransaction.RewardResult, error) {
	if reward == nil {
		return nil, status.Error(codes.FailedPrecondition, "reward is required")
	}
	amount := int64(reward.GetAmount())
	if amount <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "reward amount must be positive")
	}
	if reward.GetResourceType() == penum.ResourceType_ResourceType_Costume {
		return grantExchangeCostume(ctx, suid, reward.GetResourceId(), amount, updated)
	}
	result, err := grantExchangeReward(ctx, suid, reward, amount, updated)
	if err != nil {
		return nil, err
	}
	return []*ptransaction.RewardResult{result}, nil
}

// GrantExchangeReward preserves the single-result API for callers which do
// not expose reward results. Multi-result callers should use
// GrantExchangeRewards so dependent grants are included in their response.
func GrantExchangeReward(ctx context.Context, suid string, reward *pmaster.RewardResource, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	results, err := GrantExchangeRewards(ctx, suid, reward, updated)
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

func grantExchangeReward(ctx context.Context, suid string, reward *pmaster.RewardResource, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	switch reward.GetResourceType() {
	case penum.ResourceType_ResourceType_Item:
		return grantExchangeItem(ctx, suid, reward.GetResourceId(), amount, updated)
	case penum.ResourceType_ResourceType_Accessory:
		return grantExchangeAccessory(ctx, suid, reward.GetResourceId(), amount, updated)
	case penum.ResourceType_ResourceType_Stone, penum.ResourceType_ResourceType_PaidStone:
		return grantExchangeBalance(ctx, suid, reward.GetResourceType(), amount, updated)
	case penum.ResourceType_ResourceType_CardRarityExp,
		penum.ResourceType_ResourceType_CardRarityExpLimitChange:
		return grantExchangeCardRarityExp(ctx, suid, reward.GetResourceType(), reward.GetResourceId(), amount, updated)
	case penum.ResourceType_ResourceType_PhotoRecipe:
		return grantExchangePhotoRecipe(ctx, suid, reward.GetResourceId(), amount, updated)
	case penum.ResourceType_ResourceType_ShowcaseToy:
		return grantExchangeShowcaseToy(ctx, suid, reward.GetResourceId(), amount, updated)
	case penum.ResourceType_ResourceType_ShowcaseMusic:
		return grantExchangeShowcaseMusic(ctx, suid, reward.GetResourceId(), amount, updated)
	case penum.ResourceType_ResourceType_PhotoPose:
		return grantExchangePhotoPose(ctx, suid, reward.GetResourceId(), amount, updated)
	case penum.ResourceType_ResourceType_PhotoAllInOne:
		return grantExchangePhotoAllInOne(ctx, suid, reward.GetResourceId(), amount, updated)
	case penum.ResourceType_ResourceType_Emblem:
		return grantExchangeEmblem(ctx, suid, reward.GetResourceId(), amount, updated)
	case penum.ResourceType_ResourceType_Decoration:
		return grantExchangeDecoration(ctx, suid, reward.GetResourceId(), amount, updated)
	case penum.ResourceType_ResourceType_Hair:
		return grantExchangeHair(ctx, suid, reward.GetResourceId(), amount, updated)
	case penum.ResourceType_ResourceType_CompanyPoint:
		return GrantCompanyPoint(ctx, suid, reward.GetResourceId(), amount, updated)
	case penum.ResourceType_ResourceType_HierarchyFixedPoint,
		penum.ResourceType_ResourceType_HierarchyVariablePoint:
		return grantHierarchyPoint(ctx, suid, reward.GetResourceType(), amount, updated)
	default:
		pointType, ok := exchangePointType(reward.GetResourceType())
		if !ok {
			return nil, status.Errorf(codes.Unimplemented, "exchange reward resource type %s is unsupported", reward.GetResourceType())
		}
		return grantExchangePoint(ctx, suid, reward.GetResourceType(), pointType, amount, updated)
	}
}

func grantExchangeHair(ctx context.Context, suid, hairID string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	if amount != 1 {
		return nil, status.Error(codes.FailedPrecondition, "hair rewards must have amount one")
	}
	if _, err := opmaster.FindHair(ctx, hairID); err != nil {
		return nil, invalidExchangeMasterResource("hair", hairID, err)
	}
	if _, err := opuser.FindUserHair(ctx, suid, hairID); err == nil {
		return exchangeRewardResult(penum.ResourceType_ResourceType_Hair, hairID, 1, 1, 0), nil
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	hair := &ptransaction.UserHair{HairId: hairID}
	if err := opuser.UpsertUserHair(ctx, suid, hair); err != nil {
		return nil, err
	}
	updated.Hairs = append(updated.Hairs, hair)
	return exchangeRewardResult(penum.ResourceType_ResourceType_Hair, hairID, 0, 1, 1), nil
}

func grantExchangeEmblem(ctx context.Context, suid, emblemID string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	if amount != 1 {
		return nil, status.Error(codes.FailedPrecondition, "emblem rewards must have amount one")
	}
	if _, err := opmaster.FindEmblem(ctx, emblemID); err != nil {
		return nil, invalidExchangeMasterResource("emblem", emblemID, err)
	}
	if _, err := opuser.FindUserEmblem(ctx, suid, emblemID); err == nil {
		return exchangeRewardResult(penum.ResourceType_ResourceType_Emblem, emblemID, 1, 1, 0), nil
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	emblem := &ptransaction.UserEmblem{EmblemId: emblemID}
	if err := opuser.UpsertUserEmblem(ctx, suid, emblem); err != nil {
		return nil, err
	}
	updated.Emblems = append(updated.Emblems, emblem)
	return exchangeRewardResult(penum.ResourceType_ResourceType_Emblem, emblemID, 0, 1, 1), nil
}

func grantExchangeDecoration(ctx context.Context, suid, decorationID string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	if amount != 1 {
		return nil, status.Error(codes.FailedPrecondition, "decoration rewards must have amount one")
	}
	if _, err := opmaster.FindDecoration(ctx, decorationID); err != nil {
		return nil, invalidExchangeMasterResource("decoration", decorationID, err)
	}
	if _, err := opuser.FindUserDecoration(ctx, suid, decorationID); err == nil {
		return exchangeRewardResult(penum.ResourceType_ResourceType_Decoration, decorationID, 1, 1, 0), nil
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	decoration := &ptransaction.UserDecoration{DecorationId: decorationID}
	if err := opuser.UpsertUserDecoration(ctx, suid, decoration); err != nil {
		return nil, err
	}
	updated.Decorations = append(updated.Decorations, decoration)
	return exchangeRewardResult(penum.ResourceType_ResourceType_Decoration, decorationID, 0, 1, 1), nil
}

func grantExchangeItem(ctx context.Context, suid, itemId string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	return grantExchangeItemWithExpiration(ctx, suid, itemId, amount, 0, updated)
}

func grantExchangeItemWithExpiration(ctx context.Context, suid, itemId string, amount, expiredTime int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	if _, err := opmaster.FindItem(ctx, itemId); err != nil {
		return nil, invalidExchangeMasterResource("item", itemId, err)
	}
	items, err := opuser.FindUserItems(ctx, suid, itemId)
	if err != nil {
		return nil, err
	}
	var before int64
	var item *ptransaction.UserItem
	now := time.Now().UnixMilli()
	for _, candidate := range items {
		if candidate.Amount < 0 {
			return nil, status.Error(codes.FailedPrecondition, "item amount is invalid")
		}
		if candidate.ExpiredTime == 0 || candidate.ExpiredTime > now {
			if before > math.MaxInt64-candidate.Amount {
				return nil, status.Error(codes.FailedPrecondition, "total item amount overflows")
			}
			before += candidate.Amount
		}
		if candidate.ExpiredTime == expiredTime {
			item = candidate
		}
	}
	if item == nil {
		item = &ptransaction.UserItem{ItemId: itemId, ExpiredTime: expiredTime}
	}
	if item.Amount > math.MaxInt64-amount || before > math.MaxInt64-amount {
		return nil, status.Error(codes.FailedPrecondition, "item amount overflows")
	}
	item.Amount += amount
	if err := opuser.UpsertUserItem(ctx, suid, item); err != nil {
		return nil, err
	}
	updated.Items = append(updated.Items, item)
	return exchangeRewardResult(penum.ResourceType_ResourceType_Item, itemId, before, before+amount, amount), nil
}

func grantExchangeAccessory(ctx context.Context, suid, accessoryId string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	if _, err := opmaster.FindAccessory(ctx, accessoryId); err != nil {
		return nil, invalidExchangeMasterResource("accessory", accessoryId, err)
	}
	accessory, err := opuser.FindUserAccessory(ctx, suid, accessoryId)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		accessory = &ptransaction.UserAccessory{AccessoryId: accessoryId}
	}
	before := accessory.Amount
	if before > math.MaxInt64-amount {
		return nil, status.Error(codes.FailedPrecondition, "accessory amount overflows")
	}
	accessory.Amount += amount
	if err := opuser.UpsertUserAccessory(ctx, suid, accessory); err != nil {
		return nil, err
	}
	updated.Accessories = append(updated.Accessories, accessory)
	return exchangeRewardResult(penum.ResourceType_ResourceType_Accessory, accessoryId, before, accessory.Amount, amount), nil
}

func grantExchangeCostume(ctx context.Context, suid, costumeId string, amount int64, updated *papi.UpdatedData) ([]*ptransaction.RewardResult, error) {
	if amount != 1 {
		return nil, status.Error(codes.FailedPrecondition, "costume rewards must have amount one")
	}
	masterCostume, err := opmaster.FindCostume(ctx, costumeId)
	if err != nil {
		return nil, invalidExchangeMasterResource("costume", costumeId, err)
	}
	costume, err := opuser.FindUserCostume(ctx, suid, costumeId)
	isNew := false
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		costume, err = usercostume.SpawnUserCostumeById(ctx, costumeId)
		if err != nil {
			return nil, invalidExchangeMasterResource("costume", costumeId, err)
		}
		isNew = true
		if err := opuser.UpsertUserCostume(ctx, suid, costume); err != nil {
			return nil, err
		}
		updated.Costumes = append(updated.Costumes, costume)
	}
	before := int64(1)
	delta := int64(0)
	if isNew {
		before, delta = 0, 1
	}
	results := []*ptransaction.RewardResult{
		exchangeRewardResult(penum.ResourceType_ResourceType_Costume, costumeId, before, 1, delta),
	}
	if !isNew || masterCostume.GetDefaultHairId() == "" {
		return results, nil
	}
	hairResult, err := grantCostumeDefaultHair(ctx, suid, masterCostume.GetDefaultHairId(), updated)
	if err != nil {
		return nil, err
	}
	if hairResult != nil {
		results = append(results, hairResult)
	}
	return results, nil
}

func grantCostumeDefaultHair(ctx context.Context, suid, hairID string, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	if _, err := opmaster.FindHair(ctx, hairID); err != nil {
		return nil, invalidExchangeMasterResource("hair", hairID, err)
	}
	if _, err := opuser.FindUserHair(ctx, suid, hairID); err == nil {
		return nil, nil
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	hair := &ptransaction.UserHair{HairId: hairID, Checked: true}
	if err := opuser.UpsertUserHair(ctx, suid, hair); err != nil {
		return nil, err
	}
	updated.Hairs = append(updated.Hairs, hair)
	return exchangeRewardResult(penum.ResourceType_ResourceType_Hair, hairID, 0, 1, 1), nil
}

func grantExchangeBalance(ctx context.Context, suid string, resourceType penum.ResourceType, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	if amount > math.MaxInt32 {
		return nil, status.Error(codes.FailedPrecondition, "stone amount overflows")
	}
	balance, err := opuser.FindUserBalance(ctx, suid)
	if err != nil {
		return nil, err
	}
	var before, after int64
	if resourceType == penum.ResourceType_ResourceType_PaidStone {
		before = int64(balance.PaidBalance)
		if before+amount > math.MaxInt32 {
			return nil, status.Error(codes.FailedPrecondition, "paid stone amount overflows")
		}
		balance.PaidBalance += int32(amount)
		after = int64(balance.PaidBalance)
	} else {
		before = int64(balance.FreeBalance)
		if before+amount > math.MaxInt32 {
			return nil, status.Error(codes.FailedPrecondition, "stone amount overflows")
		}
		balance.FreeBalance += int32(amount)
		after = int64(balance.FreeBalance)
	}
	if err := opuser.UpsertUserBalance(ctx, suid, balance); err != nil {
		return nil, err
	}
	updated.Balance = balance
	return exchangeRewardResult(resourceType, "", before, after, amount), nil
}

func grantExchangeCardRarityExp(ctx context.Context, suid string, resourceType penum.ResourceType, cardId string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	if _, err := opmaster.FindCard(ctx, cardId); err != nil {
		return nil, invalidExchangeMasterResource("card", cardId, err)
	}
	card, err := opuser.FindUserCard(ctx, suid, cardId)
	if err != nil {
		return nil, err
	}
	before := int64(card.RarityTotalExp)
	if before+amount > math.MaxInt32 {
		return nil, status.Error(codes.FailedPrecondition, "card rarity exp overflows")
	}
	card.RarityTotalExp += int32(amount)
	if err := opuser.UpsertUserCard(ctx, suid, card); err != nil {
		return nil, err
	}
	putUpdatedRewardCard(updated, card)
	return exchangeRewardResult(resourceType, cardId, before, before+amount, amount), nil
}

func putUpdatedRewardCard(updated *papi.UpdatedData, card *ptransaction.UserCard) {
	for index, existing := range updated.GetCards() {
		if existing.GetCardId() == card.GetCardId() {
			updated.Cards[index] = card
			return
		}
	}
	updated.Cards = append(updated.Cards, card)
}

func grantExchangePhotoRecipe(ctx context.Context, suid, recipeId string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	if _, err := opmaster.FindPhotoRecipe(ctx, recipeId); err != nil {
		return nil, invalidExchangeMasterResource("photo recipe", recipeId, err)
	}
	recipe, err := opuser.FindUserPhotoRecipe(ctx, suid, recipeId)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		recipe = &ptransaction.UserPhotoRecipe{PhotoRecipeId: recipeId}
	}
	before := recipe.Amount
	owned, capacity, err := FindPhotoRecipeCapacity(ctx, suid)
	if err != nil {
		return nil, err
	}
	grantedAmount := min(amount, max(int64(0), capacity-owned))
	if before > math.MaxInt64-grantedAmount {
		return nil, status.Error(codes.FailedPrecondition, "photo recipe amount overflows")
	}
	if grantedAmount > 0 {
		recipe.Amount += grantedAmount
		if err := opuser.UpsertUserPhotoRecipe(ctx, suid, recipe); err != nil {
			return nil, err
		}
		updated.PhotoRecipes = append(updated.PhotoRecipes, recipe)
	}
	overflowAmount := amount - grantedAmount
	if overflowAmount > 0 {
		if err := persistPhotoCapacityGift(
			ctx, suid, penum.ResourceType_ResourceType_PhotoRecipe, recipeId,
			overflowAmount, photoRecipeCapacityGiftMessage, updated,
		); err != nil {
			return nil, err
		}
	}
	result := exchangeRewardResult(
		penum.ResourceType_ResourceType_PhotoRecipe, recipeId,
		before, recipe.Amount, amount,
	)
	result.IsGift = overflowAmount > 0
	return result, nil
}

func grantExchangeShowcaseToy(ctx context.Context, suid, toyId string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	masterToy, err := opmaster.FindShowcaseToy(ctx, toyId)
	if err != nil {
		return nil, invalidExchangeMasterResource("showcase toy", toyId, err)
	}
	toy, err := opuser.FindUserShowcaseToy(ctx, suid, toyId)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		toy = &ptransaction.UserShowcaseToy{ShowcaseToyId: toyId}
	}
	before := toy.Amount
	if masterToy.GetCategoryId() == "toy-category-figure" {
		if before > 0 {
			return exchangeRewardResult(penum.ResourceType_ResourceType_ShowcaseToy, toyId, before, before, 0), nil
		}
		amount = 1
	}
	if before > math.MaxInt64-amount {
		return nil, status.Error(codes.FailedPrecondition, "showcase toy amount overflows")
	}
	toy.Amount += amount
	if err := opuser.UpsertUserShowcaseToy(ctx, suid, toy); err != nil {
		return nil, err
	}
	updated.ShowcaseToys = append(updated.ShowcaseToys, toy)
	return exchangeRewardResult(penum.ResourceType_ResourceType_ShowcaseToy, toyId, before, toy.Amount, amount), nil
}

func grantExchangeShowcaseMusic(ctx context.Context, suid, musicID string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	if amount != 1 {
		return nil, status.Error(codes.FailedPrecondition, "showcase music rewards must have amount one")
	}
	if _, err := opmaster.FindShowcaseMusic(ctx, musicID); err != nil {
		return nil, invalidExchangeMasterResource("showcase music", musicID, err)
	}
	if _, err := opuser.FindUserShowcaseMusic(ctx, suid, musicID); err == nil {
		return exchangeRewardResult(penum.ResourceType_ResourceType_ShowcaseMusic, musicID, 1, 1, 0), nil
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	music := &ptransaction.UserShowcaseMusic{ShowcaseMusicId: musicID}
	if err := opuser.UpsertUserShowcaseMusic(ctx, suid, music); err != nil {
		return nil, err
	}
	updated.ShowcaseMusics = append(updated.ShowcaseMusics, music)
	return exchangeRewardResult(penum.ResourceType_ResourceType_ShowcaseMusic, musicID, 0, 1, 1), nil
}

func grantExchangePhotoPose(ctx context.Context, suid, poseId string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	if amount != 1 {
		return nil, status.Error(codes.FailedPrecondition, "photo pose rewards must have amount one")
	}
	if _, err := opmaster.FindPhotoPose(ctx, poseId); err != nil {
		return nil, invalidExchangeMasterResource("photo pose", poseId, err)
	}
	_, err := opuser.FindUserPhotoPose(ctx, suid, poseId)
	if err == nil {
		return exchangeRewardResult(penum.ResourceType_ResourceType_PhotoPose, poseId, 1, 1, 0), nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	pose := &ptransaction.UserPhotoPose{PhotoPoseId: poseId}
	if err := opuser.UpsertUserPhotoPose(ctx, suid, pose); err != nil {
		return nil, err
	}
	updated.PhotoPoses = append(updated.PhotoPoses, pose)
	return exchangeRewardResult(penum.ResourceType_ResourceType_PhotoPose, poseId, 0, 1, 1), nil
}

func invalidExchangeMasterResource(kind, id string, err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return status.Errorf(codes.FailedPrecondition, "%s %q is missing from master data", kind, id)
	}
	return err
}

func grantExchangePhotoAllInOne(ctx context.Context, suid, masterPhotoId string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	masterPhoto, err := opmaster.FindPhotoAllInOne(ctx, masterPhotoId)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, status.Errorf(codes.FailedPrecondition, "photo %q is missing from master data", masterPhotoId)
		}
		return nil, err
	}

	owned, capacity, err := FindPhotoCapacity(ctx, suid)
	if err != nil {
		return nil, err
	}
	photos, err := opuser.FindAllUserPhotos(ctx, suid)
	if err != nil {
		return nil, err
	}
	var before int64
	for _, photo := range photos {
		if photo.GetImageType() == penum.PhotoImageType_PhotoImageType_Asset &&
			photo.GetAssetId() == masterPhoto.GetAssetId() {
			before++
		}
	}
	grantedAmount := min(amount, max(int64(0), capacity-owned))
	for range grantedAmount {
		photo := userphoto.NewUserPhotoFromAllInOne(masterPhoto)
		if err := opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
			return nil, err
		}
		updated.Photos = append(updated.Photos, photo)
	}
	overflowAmount := amount - grantedAmount
	if overflowAmount > 0 {
		if err := persistPhotoCapacityGift(
			ctx, suid, penum.ResourceType_ResourceType_PhotoAllInOne, masterPhotoId,
			overflowAmount, PhotoCapacityGiftMessage, updated,
		); err != nil {
			return nil, err
		}
	}
	result := exchangeRewardResult(
		penum.ResourceType_ResourceType_PhotoAllInOne, masterPhotoId,
		before, before+grantedAmount, amount,
	)
	result.IsGift = overflowAmount > 0
	return result, nil
}

func persistPhotoCapacityGift(
	ctx context.Context,
	suid string,
	resourceType penum.ResourceType,
	resourceID string,
	amount int64,
	message string,
	updated *papi.UpdatedData,
) error {
	if amount <= 0 {
		return status.Error(codes.FailedPrecondition, "photo capacity gift amount is invalid")
	}
	if err := opuser.UpsertUserGift(ctx, suid, &ptransaction.UserGift{
		GiftId:       uuid.NewString(),
		ResourceType: resourceType,
		ResourceId:   resourceID,
		Amount:       amount,
		Message:      message,
		PostedTime:   time.Now().UnixMilli(),
	}); err != nil {
		return err
	}
	return SetGiftNotification(ctx, suid, true, updated)
}

func grantExchangePoint(ctx context.Context, suid string, resourceType penum.ResourceType, pointType penum.PointType, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	point, err := opuser.FindUserPoint(ctx, suid, pointType)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		point = &ptransaction.UserPoint{PointType: pointType}
	}
	before := point.Amount
	if before > math.MaxInt64-amount {
		return nil, status.Error(codes.FailedPrecondition, "point amount overflows")
	}
	point.Amount += amount
	if err := opuser.UpsertUserPoint(ctx, suid, point); err != nil {
		return nil, err
	}
	updated.Points = append(updated.Points, point)
	if resourceType == penum.ResourceType_ResourceType_Gold {
		totalCount, err := opuser.FindUserTotalCount(ctx, suid)
		if err != nil {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return nil, err
			}
			totalCount = &ptransaction.UserTotalCount{}
		}
		if totalCount.PointGoldAmount > math.MaxInt64-amount {
			return nil, status.Error(codes.FailedPrecondition, "total earned gold overflows")
		}
		totalCount.PointGoldAmount += amount
		if err := opuser.UpsertUserTotalCount(ctx, suid, totalCount); err != nil {
			return nil, err
		}
		updated.TotalCount = totalCount
		missions, err := AdvanceUserMissions(ctx, suid, penum.MissionType_MissionType_PointGoldAmount, amount)
		if err != nil {
			return nil, err
		}
		updated.Missions = append(updated.Missions, missions...)
	}
	return exchangeRewardResult(resourceType, "", before, point.Amount, amount), nil
}

func grantHierarchyPoint(
	ctx context.Context,
	suid string,
	resourceType penum.ResourceType,
	amount int64,
	updated *papi.UpdatedData,
) (*ptransaction.RewardResult, error) {
	hierarchy, err := opuser.FindUserHierarchy(ctx, suid)
	if err != nil {
		return nil, err
	}
	if _, err := HierarchyTotalPoint(hierarchy); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}

	var beforeResource int64
	switch resourceType {
	case penum.ResourceType_ResourceType_HierarchyFixedPoint:
		beforeResource = hierarchy.GetCurrentFixedPoint()
		if beforeResource > math.MaxInt64-amount {
			return nil, status.Error(codes.FailedPrecondition, "hierarchy fixed point overflows")
		}
		hierarchy.CurrentFixedPoint += amount
	case penum.ResourceType_ResourceType_HierarchyVariablePoint:
		beforeResource = hierarchy.GetCurrentVariablePoint()
		if beforeResource > math.MaxInt64-amount {
			return nil, status.Error(codes.FailedPrecondition, "hierarchy variable point overflows")
		}
		hierarchy.CurrentVariablePoint += amount
	default:
		return nil, status.Error(codes.Internal, "invalid hierarchy point resource type")
	}

	afterTotal, err := HierarchyTotalPoint(hierarchy)
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	if afterTotal > hierarchy.GetBestTotalPoint() {
		hierarchy.BestTotalPoint = afterTotal
	}
	hierarchy.CurrentPointUpdateDatetime = time.Now().UnixMilli()
	if err := opuser.UpsertUserHierarchy(ctx, suid, hierarchy); err != nil {
		return nil, err
	}
	_, reconciled, _, err := ReconcileHierarchyNotification(ctx, suid, 1)
	if err != nil {
		return nil, err
	}
	updated.Hierarchy = reconciled
	return exchangeRewardResult(resourceType, "", beforeResource, beforeResource+amount, amount), nil
}

func exchangeRewardResult(resourceType penum.ResourceType, resourceId string, before, after, amount int64) *ptransaction.RewardResult {
	return &ptransaction.RewardResult{
		ResourceType: resourceType,
		ResourceId:   resourceId,
		BeforeAmount: before,
		AfterAmount:  after,
		AmountI64:    amount,
		IsNew:        before == 0 && after > 0,
	}
}

func exchangePointType(resourceType penum.ResourceType) (penum.PointType, bool) {
	switch resourceType {
	case penum.ResourceType_ResourceType_Gold:
		return penum.PointType_PointType_Gold, true
	case penum.ResourceType_ResourceType_PvpMedal:
		return penum.PointType_PointType_PvpMedal, true
	case penum.ResourceType_ResourceType_GuildMedal:
		return penum.PointType_PointType_GuildMedal, true
	case penum.ResourceType_ResourceType_AnimeStoryPoint:
		return penum.PointType_PointType_AnimeStory, true
	case penum.ResourceType_ResourceType_GameStoryPoint:
		return penum.PointType_PointType_GameStory, true
	case penum.ResourceType_ResourceType_GroupStoryPoint:
		return penum.PointType_PointType_GroupStory, true
	case penum.ResourceType_ResourceType_ActivatePhotoShootingStoryTicket:
		return penum.PointType_PointType_ActivatePhotoShootingStoryTicket, true
	default:
		return penum.PointType_PointType_Unknown, false
	}
}
