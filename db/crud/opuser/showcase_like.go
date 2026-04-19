package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserShowcaseLike(ctx context.Context, suid string) (*ptransaction.UserShowcaseLike, error) {
	return ucommon.FindOne[ptransaction.UserShowcaseLike](
		ctx,
		uname.UserShowcaseLike,
		bson.M{"SUID": suid},
	)
}

func UpsertUserShowcaseLike(ctx context.Context, suid string, showcaseLike *ptransaction.UserShowcaseLike) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserShowcaseLike,
		bson.M{"SUID": suid},
		&ustruct.UserStruct[*ptransaction.UserShowcaseLike]{
			SUID: suid,
			Data: showcaseLike,
		},
	)
}

func DeleteUserShowcaseLike(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserShowcaseLike,
		bson.M{"SUID": suid},
	)
}
