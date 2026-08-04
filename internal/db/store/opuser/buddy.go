package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/def/user/ustruct"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

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
