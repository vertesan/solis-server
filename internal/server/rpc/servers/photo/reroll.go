package photo

import (
	"context"
	"fmt"
	"math/rand/v2"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *PhotoServer) Reroll(ctx context.Context, req *papi.PhotoRerollRequest) (*papi.PhotoRerollResponse, error) {
	if req.GetPhotoId() == "" {
		return nil, viber.InvalidArgumentf("photoId is required")
	}
	suid := viber.GetSuid(ctx).String()
	setting, err := viber.PhotoSetting(ctx)
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
		traits, err := randomRerollTraits(
			ctx, generator, photo.GetRarity(), photo.GetLevel(), user.GetManagerLevel(),
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

// randomRerollTraits replaces every randomized option, including the
// character restriction. The ordinary dedicated-photo chance is reused
// because the official reroll-specific distribution is not available.
func randomRerollTraits(
	ctx context.Context,
	g *viber.PhotoGenerator,
	rarity int32,
	level int32,
	managerLevel int32,
) (*viber.GeneratedPhotoTraits, error) {
	dedicated := rarity == 5 && len(g.DedicatedCharacterIDs) > 0 &&
		rand.IntN(100) < viber.OrdinaryDedicatedPhotoChancePercent
	characterID := ""
	if dedicated {
		characterID = g.DedicatedCharacterIDs[rand.IntN(len(g.DedicatedCharacterIDs))]
	}
	abilities, usedDedicated, err := g.RandomAbilities(ctx, rarity, level, characterID, dedicated)
	if err != nil {
		return nil, err
	}
	viber.ConfigureDedicatedPhotoAbility(abilities, usedDedicated, managerLevel)
	return &viber.GeneratedPhotoTraits{
		Rarity:      rarity,
		Level:       level,
		Abilities:   abilities,
		Dedicated:   usedDedicated,
		CharacterID: characterID,
	}, nil
}
