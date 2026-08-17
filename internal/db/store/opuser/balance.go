package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

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
		&umodel.UserStruct[*ptransaction.UserBalance]{
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
