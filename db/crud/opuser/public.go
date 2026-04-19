package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserPublic(ctx context.Context, suid string) (*ptransaction.UserPublic, error) {
	return ucommon.FindOne[ptransaction.UserPublic](
		ctx,
		uname.UserPublic,
		bson.M{"serverUserId": suid},
	)
}

func UpsertUserPublic(ctx context.Context, userPublic *ptransaction.UserPublic) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserPublic,
		bson.M{"serverUserId": userPublic.ServerUserId},
		userPublic,
	)
}

func DeleteUserPublic(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPublic,
		bson.M{"serverUserId": suid},
	)
}
