package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

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
		&umodel.UserStruct[*ptransaction.UserHair]{
			SUID: suid,
			Data: hair,
		},
	)
}

func UpsertManyUserHairs(ctx context.Context, suid string, hairs []*ptransaction.UserHair) error {
	return ucommon.ForEachAtomically(ctx, hairs, func(ctx context.Context, hair *ptransaction.UserHair) error {
		return UpsertUserHair(ctx, suid, hair)
	})
}

func DeleteAllUserHairs(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserHair,
		bson.M{"SUID": suid},
	)
}
