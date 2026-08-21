package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserCharacter(ctx context.Context, suid string, characterId string) (*ptransaction.UserCharacter, error) {
	return ucommon.FindOne[ptransaction.UserCharacter](
		ctx,
		uname.UserCharacter,
		bson.M{"SUID": suid, "characterId": characterId},
	)
}

func FindAllUserCharacters(ctx context.Context, suid string) ([]*ptransaction.UserCharacter, error) {
	return ucommon.FindAll[ptransaction.UserCharacter](
		ctx,
		uname.UserCharacter,
		bson.M{"SUID": suid},
	)
}

func UpsertUserCharacter(ctx context.Context, suid string, character *ptransaction.UserCharacter) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserCharacter,
		bson.M{
			"SUID":        suid,
			"characterId": character.CharacterId,
		},
		&umodel.UserStruct[*ptransaction.UserCharacter]{
			SUID: suid,
			Data: character,
		},
	)
}

func UpsertManyUserCharacters(ctx context.Context, suid string, characters []*ptransaction.UserCharacter) error {
	return ucommon.ForEachAtomically(ctx, characters, func(ctx context.Context, character *ptransaction.UserCharacter) error {
		return UpsertUserCharacter(ctx, suid, character)
	})
}

func DeleteAllUserCharacters(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserCharacter,
		bson.M{"SUID": suid},
	)
}
