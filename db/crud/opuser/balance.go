package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserBalance(ctx context.Context, suid string) (*ptransaction.UserBalance, error) {
	return ucommon.FindOne[ptransaction.UserBalance](
		ctx,
		uname.UserBalance,
		bson.M{"SUID": suid},
	)
}

func UpsertUserBalance(ctx context.Context, suid string, balance *ptransaction.UserBalance) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserBalance,
		bson.M{"SUID": suid},
		&ustruct.UserStruct[*ptransaction.UserBalance]{
			SUID: suid,
			Data: balance,
		},
	)
}

func DeleteUserBalance(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserBalance,
		bson.M{"SUID": suid},
	)
}
