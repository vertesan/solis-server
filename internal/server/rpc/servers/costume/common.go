package costume

import (
	"context"
	"slices"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func setCharacterCostume(ctx context.Context, costumeId, hairId string, live bool) (*papi.UpdatedData, error) {
	suid := viber.GetSuid(ctx).String()
	var updated *papi.UpdatedData
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		attemptUpdated := &papi.UpdatedData{}
		masterCostume, err := opmaster.FindCostume(ctx, costumeId)
		if err != nil {
			return nil, err
		}
		masterHair, err := opmaster.FindHair(ctx, hairId)
		if err != nil {
			return nil, err
		}
		if masterCostume.CharacterId == "" || masterCostume.CharacterId != masterHair.CharacterId {
			return nil, viber.FailedPreconditionf("costume and hair belong to different characters")
		}
		if slices.Contains(masterHair.NotWearableCostumeIds, costumeId) ||
			len(masterHair.WearableCostumeIds) > 0 && !slices.Contains(masterHair.WearableCostumeIds, costumeId) {
			return nil, viber.FailedPreconditionf("hair cannot be worn with costume")
		}
		costume, err := opuser.FindUserCostume(ctx, suid, costumeId)
		if err != nil {
			return nil, err
		}
		hair, err := opuser.FindUserHair(ctx, suid, hairId)
		if err != nil {
			return nil, err
		}
		character, err := opuser.FindUserCharacter(ctx, suid, masterCostume.CharacterId)
		if err != nil {
			return nil, err
		}
		if live {
			character.LiveCostumeId, character.LiveHairId = costumeId, hairId
		} else {
			character.FavoriteCostumeId, character.FavoriteHairId = costumeId, hairId
		}
		if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
			return nil, err
		}
		attemptUpdated.Characters = append(attemptUpdated.Characters, character)
		if live && costume.Checked {
			costume.Checked = false
			if err := opuser.UpsertUserCostume(ctx, suid, costume); err != nil {
				return nil, err
			}
			attemptUpdated.Costumes = append(attemptUpdated.Costumes, costume)
		}
		if !live && hair.Checked {
			hair.Checked = false
			if err := opuser.UpsertUserHair(ctx, suid, hair); err != nil {
				return nil, err
			}
			attemptUpdated.Hairs = append(attemptUpdated.Hairs, hair)
		}
		updated = attemptUpdated
		return nil, nil
	})
	return updated, err
}
