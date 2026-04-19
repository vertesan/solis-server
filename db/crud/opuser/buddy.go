package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserBuddy(ctx context.Context, suid string) (*ptransaction.UserBuddy, error) {
	return ucommon.FindOne[ptransaction.UserBuddy](
		ctx,
		uname.UserBuddy,
		bson.M{"SUID": suid},
	)
}

func UpsertUserBuddy(ctx context.Context, suid string, buddy *ptransaction.UserBuddy) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserBuddy,
		bson.M{"SUID": suid},
		&ustruct.UserStruct[*ptransaction.UserBuddy]{
			SUID: suid,
			Data: buddy,
		},
	)
}

func DeleteUserBuddy(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserBuddy,
		bson.M{"SUID": suid},
	)
}
