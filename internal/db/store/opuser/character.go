package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/def/user/ustruct"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

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
