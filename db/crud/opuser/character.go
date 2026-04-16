package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func UpsertUserCharacter(ctx context.Context, suid string, character *ptransaction.UserCharacter) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserCharacter,
		bson.M{
			"SUID":        suid,
			"characterId": character.CharacterId,
		},
		&ustruct.UserStruct[*ptransaction.UserCharacter]{
			SUID: suid,
			Data: character,
		},
	)
}

func UpsertManyUserCharacters(ctx context.Context, suid string, characters []*ptransaction.UserCharacter) error {
	for _, character := range characters {
		if err := UpsertUserCharacter(ctx, suid, character); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserCharacters(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserCharacter,
		bson.M{"SUID": suid},
	)
}
