package viber

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

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
	"photo_pose":                           penum.ResourceType_ResourceType_PhotoPose,
	"photo_all_in_one":                     penum.ResourceType_ResourceType_PhotoAllInOne,
}

func ValidateExchangeResources(costType penum.ResourceType, rewards []*pmaster.RewardResource) error {
	if !isSupportedExchangeCost(costType) {
		return status.Errorf(codes.Unimplemented, "exchange cost resource type %s is unsupported", costType)
	}
	for _, reward := range rewards {
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
		penum.ResourceType_ResourceType_PhotoRecipe,
		penum.ResourceType_ResourceType_ShowcaseToy,
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
	items, err := opuser.FindUserItems(ctx, suid, itemId)
	if err != nil {
		return err
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
		return ErrExchangeInsufficientResource
	}
	remaining := amount
	for _, item := range items {
		if remaining == 0 || item.Amount <= 0 || (item.ExpiredTime > 0 && item.ExpiredTime <= now) {
			continue
		}
		consumed := min(item.Amount, remaining)
		item.Amount -= consumed
		remaining -= consumed
		if err := opuser.UpsertUserItem(ctx, suid, item); err != nil {
			return err
		}
		updated.Items = append(updated.Items, item)
	}
	return nil
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

func GrantExchangeReward(ctx context.Context, suid string, reward *pmaster.RewardResource, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	amount := int64(reward.Amount)
	switch reward.ResourceType {
	case penum.ResourceType_ResourceType_Item:
		return grantExchangeItem(ctx, suid, reward.ResourceId, amount, updated)
	case penum.ResourceType_ResourceType_Accessory:
		return grantExchangeAccessory(ctx, suid, reward.ResourceId, amount, updated)
	case penum.ResourceType_ResourceType_Costume:
		return grantExchangeCostume(ctx, suid, reward.ResourceId, amount, updated)
	case penum.ResourceType_ResourceType_Stone, penum.ResourceType_ResourceType_PaidStone:
		return grantExchangeBalance(ctx, suid, reward.ResourceType, amount, updated)
	case penum.ResourceType_ResourceType_CardRarityExp:
		return grantExchangeCardRarityExp(ctx, suid, reward.ResourceId, amount, updated)
	case penum.ResourceType_ResourceType_PhotoRecipe:
		return grantExchangePhotoRecipe(ctx, suid, reward.ResourceId, amount, updated)
	case penum.ResourceType_ResourceType_ShowcaseToy:
		return grantExchangeShowcaseToy(ctx, suid, reward.ResourceId, amount, updated)
	case penum.ResourceType_ResourceType_PhotoPose:
		return grantExchangePhotoPose(ctx, suid, reward.ResourceId, amount, updated)
	case penum.ResourceType_ResourceType_PhotoAllInOne:
		return grantExchangePhotoAllInOne(ctx, suid, reward.ResourceId, amount, updated)
	default:
		pointType, ok := exchangePointType(reward.ResourceType)
		if !ok {
			return nil, status.Errorf(codes.Unimplemented, "exchange reward resource type %s is unsupported", reward.ResourceType)
		}
		return grantExchangePoint(ctx, suid, reward.ResourceType, pointType, amount, updated)
	}
}

func grantExchangeItem(ctx context.Context, suid, itemId string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
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
		if candidate.ExpiredTime == 0 || candidate.ExpiredTime > now {
			before += candidate.Amount
		}
		if candidate.ExpiredTime == 0 {
			item = candidate
		}
	}
	if item == nil {
		item = &ptransaction.UserItem{ItemId: itemId}
	}
	if item.Amount > math.MaxInt64-amount {
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

func grantExchangeCostume(ctx context.Context, suid, costumeId string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	if amount != 1 {
		return nil, status.Error(codes.FailedPrecondition, "costume rewards must have amount one")
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
	if !isNew {
		if _, err := opmaster.FindCostume(ctx, costumeId); err != nil {
			return nil, invalidExchangeMasterResource("costume", costumeId, err)
		}
	}
	before := int64(1)
	delta := int64(0)
	if isNew {
		before, delta = 0, 1
	}
	return exchangeRewardResult(penum.ResourceType_ResourceType_Costume, costumeId, before, 1, delta), nil
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

func grantExchangeCardRarityExp(ctx context.Context, suid, cardId string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
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
	updated.Cards = append(updated.Cards, card)
	return exchangeRewardResult(penum.ResourceType_ResourceType_CardRarityExp, cardId, before, before+amount, amount), nil
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
	if before > math.MaxInt64-amount {
		return nil, status.Error(codes.FailedPrecondition, "photo recipe amount overflows")
	}
	recipe.Amount += amount
	if err := opuser.UpsertUserPhotoRecipe(ctx, suid, recipe); err != nil {
		return nil, err
	}
	updated.PhotoRecipes = append(updated.PhotoRecipes, recipe)
	return exchangeRewardResult(penum.ResourceType_ResourceType_PhotoRecipe, recipeId, before, recipe.Amount, amount), nil
}

func grantExchangeShowcaseToy(ctx context.Context, suid, toyId string, amount int64, updated *papi.UpdatedData) (*ptransaction.RewardResult, error) {
	if _, err := opmaster.FindShowcaseToy(ctx, toyId); err != nil {
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

	for range amount {
		photo := userphoto.NewUserPhotoFromAllInOne(masterPhoto)
		if err := opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
			return nil, err
		}
		updated.Photos = append(updated.Photos, photo)
	}
	return exchangeRewardResult(penum.ResourceType_ResourceType_PhotoAllInOne, masterPhotoId, 0, amount, amount), nil
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
	return exchangeRewardResult(resourceType, "", before, point.Amount, amount), nil
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
