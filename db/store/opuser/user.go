package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUser(ctx context.Context, suid string) (*ptransaction.User, error) {
	return ucommon.FindOne[ptransaction.User](
		ctx,
		uname.User,
		bson.M{"SUID": suid},
	)
}

func UpsertUser(ctx context.Context, suid string, user *ptransaction.User) error {
	return ucommon.UpsertOne(
		ctx,
		uname.User,
		bson.M{"SUID": suid},
		&ustruct.UserStruct[*ptransaction.User]{
			SUID: suid,
			Data: user,
		},
	)
}

func DeleteUser(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.User,
		bson.M{"SUID": suid},
	)
}
