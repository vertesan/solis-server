package servers

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/mcontext"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *PhotoServer) SwitchLock(ctx context.Context, req *papi.PhotoSwitchLockRequest) (*papi.PhotoSwitchLockResponse, error) {
	ids, err := uniqueRequiredPhotoIDs(req.GetPhotoIds())
	if err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoSwitchLockResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		for _, id := range ids {
			photo, err := opuser.FindUserPhoto(ctx, suid, id)
			if err != nil {
				return nil, err
			}
			photo.Locked = !photo.GetLocked()
			if err := opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
				return nil, err
			}
			updated.Photos = append(updated.Photos, photo)
		}
		response = &papi.PhotoSwitchLockResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *PhotoServer) Delete(ctx context.Context, req *papi.PhotoDeleteRequest) (*papi.PhotoDeleteResponse, error) {
	ids, err := uniqueRequiredPhotoIDs(req.GetPhotoIds())
	if err != nil {
		return nil, err
	}
	setting, err := photoSetting(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if len(ids) > int(setting.GetPhotoDeleteAmountLimit()) {
		return nil, viber.InvalidArgumentf("at most %d photos can be deleted", setting.GetPhotoDeleteAmountLimit())
	}
	suid := viber.GetSuid(ctx).String()
	assetIDs := make([]string, 0, len(ids))
	var response *papi.PhotoDeleteResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		deleted := &papi.DeletedData{}
		profile, err := opuser.FindUserProfile(ctx, suid)
		if err != nil {
			return nil, err
		}
		var rewardAmount int64
		for _, id := range ids {
			photo, err := opuser.FindUserPhoto(ctx, suid, id)
			if err != nil {
				return nil, err
			}
			if photo.GetLocked() {
				return nil, status.Errorf(codes.FailedPrecondition, "photo %q is locked", id)
			}
			if profile.GetFavoritePhotoId() == id {
				return nil, status.Errorf(codes.FailedPrecondition, "photo %q is set on the business card", id)
			}
			level, err := opmaster.FindPhotoRarityLevel(ctx, photo.GetRarity(), photo.GetLevel())
			if err != nil {
				return nil, err
			}
			rewardAmount += int64(level.GetDeleteRewardItem())
			if photo.GetImageType() == penum.PhotoImageType_PhotoImageType_User && photo.GetAssetId() != "" {
				assetIDs = append(assetIDs, photo.GetAssetId())
			}
			deleted.Photos = append(deleted.Photos, &ptransaction.UserPhoto{PhotoId: id})
		}
		if rewardAmount <= 0 || rewardAmount > math.MaxInt32 {
			return nil, fmt.Errorf("photo delete reward amount is invalid")
		}
		results, err := viber.GrantExchangeRewards(ctx, suid, &pmaster.RewardResource{
			ResourceType: penum.ResourceType_ResourceType_Item,
			ResourceId:   viber.PhotoRerollItemID,
			Amount:       int32(rewardAmount),
		}, updated)
		if err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserPhotos(ctx, suid, ids); err != nil {
			return nil, err
		}
		common := viber.NewCommonResponse(updated)
		common.DeletedData = deleted
		response = &papi.PhotoDeleteResponse{RewardResults: results, CommonResponse: common}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	removeUserPhotoImageObjects(ctx, suid, assetIDs)
	return response, nil
}

func (s *PhotoServer) Enhance(ctx context.Context, req *papi.PhotoEnhanceRequest) (*papi.PhotoEnhanceResponse, error) {
	if req.GetPhotoId() == "" || req.GetLevel() <= 0 {
		return nil, viber.InvalidArgumentf("photoId and a positive level are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoEnhanceResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		photo, err := opuser.FindUserPhoto(ctx, suid, req.GetPhotoId())
		if err != nil {
			return nil, err
		}
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		managerLevel, err := opmaster.FindManagerLevel(ctx, user.GetManagerLevel())
		if err != nil {
			return nil, err
		}
		if req.GetLevel() > managerLevel.GetMaxPhotoEnhanceLevel() {
			return nil, status.Error(codes.FailedPrecondition, "target photo level exceeds the manager-level limit")
		}
		if req.GetLevel() <= photo.GetLevel() {
			return nil, status.Error(codes.FailedPrecondition, "photo enhancement level must increase")
		}
		if _, err := opmaster.FindPhotoRarityLevel(ctx, photo.GetRarity(), req.GetLevel()); err != nil {
			return nil, status.Error(codes.FailedPrecondition, "target photo level is unsupported")
		}
		consumptions, err := photoEnhanceConsumptions(ctx, photo.GetRarity(), photo.GetPhotoRecipeId(), photo.GetLevel(), req.GetLevel())
		if err != nil {
			return nil, err
		}
		if photo.GetIsDreamBase() {
			materialLevel := photo.GetDreamMaterialPhotoLevel()
			if materialLevel <= 0 {
				materialLevel = photo.GetLevel()
			}
			materialTarget := materialLevel + req.GetLevel() - photo.GetLevel()
			more, err := photoEnhanceConsumptions(
				ctx,
				photo.GetDreamMaterialPhotoRarity(),
				photo.GetDreamMaterialPhotoRecipeId(),
				materialLevel,
				materialTarget,
			)
			if err != nil {
				return nil, err
			}
			consumptions = append(consumptions, more...)
			photo.DreamMaterialPhotoLevel = materialTarget
		}
		updated := &papi.UpdatedData{}
		if err := viber.ConsumePhotoConsumptions(ctx, suid, consumptions, updated); err != nil {
			return nil, err
		}
		photo.Level = req.GetLevel()
		if err := viber.RecalculatePhotoAbilities(ctx, photo.GetAbilities(), photo.GetLevel()); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
			return nil, err
		}
		updated.Photos = append(updated.Photos, photo)
		countMissions, err := viber.AdvanceUserMissions(ctx, suid, penum.MissionType_MissionType_PhotoLevelUpCount, 1)
		if err != nil {
			return nil, err
		}
		levelMissions, err := viber.ReachUserMissions(
			ctx, suid, penum.MissionType_MissionType_PhotoLevelAchievement, int64(photo.GetLevel()),
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = viber.AppendUniquePhotoMissions(updated.Missions, countMissions, levelMissions)
		response = &papi.PhotoEnhanceResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *PhotoServer) Reroll(ctx context.Context, req *papi.PhotoRerollRequest) (*papi.PhotoRerollResponse, error) {
	if req.GetPhotoId() == "" {
		return nil, viber.InvalidArgumentf("photoId is required")
	}
	suid := viber.GetSuid(ctx).String()
	setting, err := photoSetting(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	var response *papi.PhotoRerollResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		photo, err := opuser.FindUserPhoto(ctx, suid, req.GetPhotoId())
		if err != nil {
			return nil, err
		}
		if !photo.GetRerollable() || photo.GetIsDreamBase() {
			return nil, status.Error(codes.FailedPrecondition, "photo cannot be rerolled")
		}
		level, err := opmaster.FindPhotoRarityLevel(ctx, photo.GetRarity(), photo.GetLevel())
		if err != nil {
			return nil, err
		}
		if level.GetRerollRequiredItem() <= 0 {
			return nil, fmt.Errorf("photo reroll cost is invalid")
		}
		updated := &papi.UpdatedData{}
		if err := viber.ConsumeExchangeResource(
			ctx, suid, penum.ResourceType_ResourceType_Item, viber.PhotoRerollItemID,
			int64(level.GetRerollRequiredItem()), updated,
		); err != nil {
			return nil, err
		}
		generator, err := viber.NewPhotoGenerator(ctx, setting)
		if err != nil {
			return nil, err
		}
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		traits, err := generator.RandomRerollTraits(
			ctx, photo.GetRarity(), photo.GetLevel(), user.GetManagerLevel(),
		)
		if err != nil {
			return nil, err
		}
		photo.Abilities = traits.Abilities
		photo.CharacterIds = nil
		if traits.Dedicated {
			photo.CharacterIds = []string{traits.CharacterID}
		}
		if err := opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
			return nil, err
		}
		updated.Photos = append(updated.Photos, photo)
		missions, err := viber.AdvanceUserMissions(ctx, suid, penum.MissionType_MissionType_PhotoRerollCount, 1)
		if err != nil {
			return nil, err
		}
		updated.Missions = missions
		response = &papi.PhotoRerollResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *PhotoServer) Retouch(ctx context.Context, req *papi.PhotoRetouchRequest) (*papi.PhotoRetouchResponse, error) {
	if req.GetPhotoId() == "" || req.GetPhotoRecipeId() == "" {
		return nil, viber.InvalidArgumentf("photoId and photoRecipeId are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoRetouchResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		photo, err := opuser.FindUserPhoto(ctx, suid, req.GetPhotoId())
		if err != nil {
			return nil, err
		}
		master, err := opmaster.FindPhotoRecipe(ctx, req.GetPhotoRecipeId())
		if err != nil {
			return nil, err
		}
		if photo.GetIsDreamBase() {
			return nil, status.Error(codes.FailedPrecondition, "image-training base photos cannot be retouched")
		}
		recipe, err := opuser.FindUserPhotoRecipe(ctx, suid, req.GetPhotoRecipeId())
		if err != nil {
			return nil, err
		}
		if recipe.GetAmount() <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "photo recipe is not owned")
		}
		set, err := opmaster.FindPhotoAbilitySet(ctx, master.GetPhotoAbilitySetId())
		if err != nil {
			return nil, err
		}
		level, err := randomPhotoRetouchLevel(ctx, master, set)
		if err != nil {
			return nil, err
		}
		abilities, err := viber.BuildPhotoAbilitiesFromSet(ctx, set, level)
		if err != nil {
			return nil, err
		}
		consumption, err := opmaster.FindConsumption(ctx, master.GetRetouchConsumptionId())
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		if err := viber.ConsumePhotoConsumptions(ctx, suid, []*pmaster.Consumption{consumption}, updated); err != nil {
			return nil, err
		}
		recipe.Amount--
		if err := opuser.UpsertUserPhotoRecipe(ctx, suid, recipe); err != nil {
			return nil, err
		}
		updated.PhotoRecipes = append(updated.PhotoRecipes, recipe)
		photo.Rarity = set.GetPhotoRarity()
		photo.Level = level
		photo.Abilities = abilities
		photo.CharacterIds = append([]string(nil), master.GetCharacterIds()...)
		photo.PhotoRecipeId = master.GetId()
		photo.Rerollable = false
		photo.IsDreamBase = false
		photo.DreamMaterialPhotoRarity = 0
		photo.DreamMaterialPhotoLevel = 0
		photo.DreamMaterialPhotoRecipeId = ""
		if err := opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
			return nil, err
		}
		updated.Photos = append(updated.Photos, photo)
		total, err := opuser.FindUserTotalCount(ctx, suid)
		if err != nil {
			return nil, err
		}
		total.PhotoRetouchCount++
		if err := opuser.UpsertUserTotalCount(ctx, suid, total); err != nil {
			return nil, err
		}
		updated.TotalCount = total
		retouch, err := viber.AdvanceUserMissions(ctx, suid, penum.MissionType_MissionType_PhotoRetouchCount, 1)
		if err != nil {
			return nil, err
		}
		retouchTotal, err := viber.AdvanceUserMissions(ctx, suid, penum.MissionType_MissionType_PhotoRetouchTotalCount, 1)
		if err != nil {
			return nil, err
		}
		recipeUsed, err := viber.AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_PhotoRecipeUsedAmount, 1, master.GetId(),
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = viber.AppendUniquePhotoMissions(updated.Missions, retouch, retouchTotal, recipeUsed)
		response = &papi.PhotoRetouchResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *PhotoServer) Update(ctx context.Context, req *papi.PhotoUpdateRequest) (*papi.PhotoUpdateResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if req.GetPhotoId() == "" || name == "" {
		return nil, viber.InvalidArgumentf("photoId and name are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoUpdateResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		photo, err := opuser.FindUserPhoto(ctx, suid, req.GetPhotoId())
		if err != nil {
			return nil, err
		}
		photo.Name = name
		if err := opuser.UpsertUserPhoto(ctx, suid, photo); err != nil {
			return nil, err
		}
		response = &papi.PhotoUpdateResponse{
			CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{Photos: []*ptransaction.UserPhoto{photo}}),
		}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *PhotoServer) ExtendLimit(ctx context.Context, _ *papi.Empty) (*papi.PhotoExtendLimitResponse, error) {
	suid := viber.GetSuid(ctx).String()
	setting, err := photoSetting(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	var response *papi.PhotoExtendLimitResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		if user.GetPhotoLimitExtendedCount() >= setting.GetPhotoLimitExtendLimit() {
			return nil, status.Error(codes.FailedPrecondition, "photo capacity is already fully extended")
		}
		if setting.GetPhotoLimitExtendStone() <= 0 || setting.GetPhotoLimitExtendAmount() <= 0 {
			return nil, fmt.Errorf("photo limit extension master data is invalid")
		}
		updated := &papi.UpdatedData{}
		if err := viber.ConsumeExchangeResource(
			ctx, suid, penum.ResourceType_ResourceType_Stone, "", int64(setting.GetPhotoLimitExtendStone()), updated,
		); err != nil {
			return nil, err
		}
		user.PhotoLimitExtendedCount++
		if err := opuser.UpsertUser(ctx, suid, user); err != nil {
			return nil, err
		}
		updated.User = user
		response = &papi.PhotoExtendLimitResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *PhotoServer) SaleRecipe(ctx context.Context, req *papi.PhotoSaleRecipeRequest) (*papi.PhotoSaleRecipeResponse, error) {
	ids, err := requiredPhotoRecipeIDs(req.GetPhotoRecipeIds())
	if err != nil {
		return nil, err
	}
	setting, err := photoSetting(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	if len(ids) > int(setting.GetPhotoRecipeSaleAmountLimit()) {
		return nil, viber.InvalidArgumentf("at most %d recipes can be sold", setting.GetPhotoRecipeSaleAmountLimit())
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.PhotoSaleRecipeResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		results := make([]*ptransaction.RewardResult, 0)
		amounts := make(map[string]int64, len(ids))
		order := make([]string, 0, len(ids))
		for _, id := range ids {
			if amounts[id] == 0 {
				order = append(order, id)
			}
			amounts[id]++
		}
		for _, id := range order {
			master, err := opmaster.FindPhotoRecipe(ctx, id)
			if err != nil {
				return nil, err
			}
			recipe, err := opuser.FindUserPhotoRecipe(ctx, suid, id)
			if err != nil {
				return nil, err
			}
			amount := amounts[id]
			if recipe.GetAmount() < amount {
				return nil, status.Errorf(codes.FailedPrecondition, "photo recipe %q amount is insufficient", id)
			}
			reward, err := opmaster.FindReward(ctx, master.GetSaleRewardId())
			if err != nil {
				return nil, err
			}
			recipe.Amount -= amount
			if err := opuser.UpsertUserPhotoRecipe(ctx, suid, recipe); err != nil {
				return nil, err
			}
			updated.PhotoRecipes = append(updated.PhotoRecipes, recipe)
			for range amount {
				for _, resource := range reward.GetResources() {
					granted, err := viber.GrantExchangeRewards(ctx, suid, resource, updated)
					if err != nil {
						return nil, err
					}
					results = append(results, granted...)
				}
			}
		}
		results, err = viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}
		dedupePhotoUpdatedItems(updated)
		response = &papi.PhotoSaleRecipeResponse{RewardResults: results, CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func photoEnhanceConsumptions(
	ctx context.Context,
	rarity int32,
	recipeID string,
	currentLevel int32,
	targetLevel int32,
) ([]*pmaster.Consumption, error) {
	if targetLevel <= currentLevel || currentLevel%5 != 0 || targetLevel%5 != 0 {
		return nil, status.Error(codes.FailedPrecondition, "photo levels must increase in five-level steps")
	}
	result := make([]*pmaster.Consumption, 0, (targetLevel-currentLevel)/5)
	if recipeID == "" {
		for level := currentLevel + 5; level <= targetLevel; level += 5 {
			master, err := opmaster.FindPhotoRarityLevel(ctx, rarity, level)
			if err != nil {
				return nil, err
			}
			if master.GetEnhanceRequiredItem() <= 0 {
				return nil, fmt.Errorf("photo enhance cost is invalid")
			}
			result = append(result, &pmaster.Consumption{Resources: []*pmaster.ConsumptionResource{{
				ResourceType: penum.ResourceType_ResourceType_Item,
				ResourceIds:  []string{viber.PhotoEnhanceItemID},
				Amount:       master.GetEnhanceRequiredItem(),
			}}})
		}
		return result, nil
	}
	recipe, err := opmaster.FindPhotoRecipe(ctx, recipeID)
	if err != nil {
		return nil, err
	}
	if recipe.GetPhotoRetouchEnhanceGroupId() == "" {
		return nil, status.Error(codes.FailedPrecondition, "photo recipe has no enhancement group")
	}
	for level := currentLevel + 5; level <= targetLevel; level += 5 {
		enhance, err := opmaster.FindPhotoRetouchEnhance(ctx, recipe.GetPhotoRetouchEnhanceGroupId(), level)
		if err != nil {
			return nil, err
		}
		consumption, err := opmaster.FindConsumption(ctx, enhance.GetConsumptionId())
		if err != nil {
			return nil, err
		}
		result = append(result, consumption)
	}
	return result, nil
}

func randomPhotoRetouchLevel(
	ctx context.Context,
	recipe *pmaster.PhotoRecipe,
	set *pmaster.PhotoAbilitySet,
) (int32, error) {
	if recipe == nil || set == nil || set.GetPhotoRarity() < 1 || set.GetPhotoRarity() > 5 ||
		recipe.GetMinPhotoLevel() <= 0 || recipe.GetMaxPhotoLevel() < recipe.GetMinPhotoLevel() {
		return 0, status.Error(codes.FailedPrecondition, "photo recipe result data is invalid")
	}
	levels, err := opmaster.FindPhotoRarityLevels(ctx, set.GetPhotoRarity())
	if err != nil {
		return 0, err
	}
	candidates := make([]int32, 0, len(levels))
	for _, level := range levels {
		if level.GetLevel() >= recipe.GetMinPhotoLevel() && level.GetLevel() <= recipe.GetMaxPhotoLevel() {
			candidates = append(candidates, level.GetLevel())
		}
	}
	if len(candidates) == 0 {
		return 0, status.Error(codes.FailedPrecondition, "photo recipe has no valid result quality")
	}
	return candidates[rand.IntN(len(candidates))], nil
}

func uniqueRequiredPhotoIDs(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, viber.InvalidArgumentf("at least one photoId is required")
	}
	return uniqueRequiredStrings(values, "photoId")
}

func requiredPhotoRecipeIDs(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, viber.InvalidArgumentf("at least one photoRecipeId is required")
	}
	for _, value := range values {
		if value == "" {
			return nil, viber.InvalidArgumentf("photoRecipeId is required")
		}
	}
	return values, nil
}

func uniqueRequiredStrings(values []string, name string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			return nil, viber.InvalidArgumentf("%s is required", name)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func photoSetting(ctx context.Context) (*pmaster.Setting, error) {
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return nil, err
	}
	return opmaster.FindSetting(ctx, settingID)
}

func dedupePhotoUpdatedItems(updated *papi.UpdatedData) {
	type itemKey struct {
		id          string
		expiredTime int64
	}
	last := make(map[itemKey]int, len(updated.GetItems()))
	for index, item := range updated.GetItems() {
		last[itemKey{id: item.GetItemId(), expiredTime: item.GetExpiredTime()}] = index
	}
	items := make([]*ptransaction.UserItem, 0, len(last))
	for index, item := range updated.GetItems() {
		if last[itemKey{id: item.GetItemId(), expiredTime: item.GetExpiredTime()}] == index {
			items = append(items, item)
		}
	}
	updated.Items = items
}

func removeUserPhotoImageObjects(ctx context.Context, suid string, assetIDs []string) {
	if len(assetIDs) == 0 {
		return
	}
	auth, err := opuser.GetUserAuthByServerId(ctx, suid)
	if err != nil {
		mcontext.GetLogger(ctx).Warnw("failed to find photo owner while cleaning image objects", "error", err)
		return
	}
	names := make([]string, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		names = append(names, filepath.ToSlash(filepath.Join("Photo", auth.PublicUserId, assetID)))
	}
	metadata, err := opuser.FindUserPhotoImageMetadataByNames(ctx, suid, names)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		mcontext.GetLogger(ctx).Warnw("failed to find photo image metadata for cleanup", "error", err)
		return
	}
	removedNames := make([]string, 0, len(metadata))
	for _, object := range metadata {
		path := filepath.Join(config.GetResourceDir(), "Photo", object.PublicUserId, filepath.Base(object.Name)+".jpg")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			mcontext.GetLogger(ctx).Warnw("failed to delete photo image file", "path", path, "error", err)
			continue
		}
		removedNames = append(removedNames, object.Name)
	}
	// Keep metadata for files that could not be removed. A later cleanup can
	// then discover and retry those objects instead of leaking an untracked file.
	if err := opuser.DeleteUserPhotoImageMetadataByNames(ctx, suid, removedNames); err != nil {
		mcontext.GetLogger(ctx).Warnw("failed to delete photo image metadata", "error", err)
	}
}
