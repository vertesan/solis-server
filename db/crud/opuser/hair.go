package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserHairs(ctx context.Context, suid string) ([]*ptransaction.UserHair, error) {
	return ucommon.FindAll[ptransaction.UserHair](
		ctx,
		uname.UserHair,
		bson.M{"SUID": suid},
	)
}

func UpsertUserHair(ctx context.Context, suid string, hair *ptransaction.UserHair) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserHair,
		bson.M{"SUID": suid, "hairId": hair.HairId},
		&ustruct.UserStruct[*ptransaction.UserHair]{
			SUID: suid,
			Data: hair,
		},
	)
}

func UpsertManyUserHairs(ctx context.Context, suid string, hairs []*ptransaction.UserHair) error {
	for _, hair := range hairs {
		if err := UpsertUserHair(ctx, suid, hair); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserHairs(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserHair,
		bson.M{"SUID": suid},
	)
}
