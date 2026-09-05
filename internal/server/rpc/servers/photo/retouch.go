package photo

import (
	"context"
	"math/rand/v2"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
		if err := consumePhotoConsumptions(ctx, suid, []*pmaster.Consumption{consumption}, updated); err != nil {
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
