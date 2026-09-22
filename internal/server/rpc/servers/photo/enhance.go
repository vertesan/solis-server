package photo

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
		if err := consumePhotoConsumptions(ctx, suid, consumptions, updated); err != nil {
			return nil, err
		}
		photo.Level = req.GetLevel()
		if err := recalculatePhotoAbilities(ctx, photo.GetAbilities(), photo.GetLevel()); err != nil {
			return nil, err
		}
		if photo.GetIsDreamBase() {
			if err := dream.ReconcilePhotoEnhance(ctx, suid, photo); err != nil {
				return nil, err
			}
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

func recalculatePhotoAbilities(
	ctx context.Context,
	abilities []*ptransaction.UserPhotoAbility,
	level int32,
) error {
	ids := make([]string, 0, len(abilities))
	for _, ability := range abilities {
		if ability == nil || ability.GetPhotoAbilityId() == "" {
			return status.Error(codes.FailedPrecondition, "photo contains an invalid ability")
		}
		ids = append(ids, ability.GetPhotoAbilityId())
	}
	masters, err := opmaster.FindPhotoAbilities(ctx, ids)
	if err != nil {
		return err
	}
	byID := make(map[string]*pmaster.PhotoAbility, len(masters))
	for _, master := range masters {
		byID[master.GetId()] = master
	}
	for _, ability := range abilities {
		master := byID[ability.GetPhotoAbilityId()]
		if master == nil {
			return status.Errorf(codes.FailedPrecondition, "photo ability %q is absent", ability.GetPhotoAbilityId())
		}
		value, err := viber.PhotoAbilityEffectValue(master, level, ability.GetGrade())
		if err != nil {
			return err
		}
		ability.EffectValue = value
	}
	return nil
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
