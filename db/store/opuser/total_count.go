package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserTotalCount(ctx context.Context, suid string) (*ptransaction.UserTotalCount, error) {
	return ucommon.FindOne[ptransaction.UserTotalCount](
		ctx,
		uname.UserTotalCount,
		bson.M{"SUID": suid},
	)
}

func UpsertUserTotalCount(ctx context.Context, suid string, totalCount *ptransaction.UserTotalCount) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserTotalCount,
		bson.M{"SUID": suid},
		&ustruct.UserStruct[*ptransaction.UserTotalCount]{
			SUID: suid,
			Data: totalCount,
		},
	)
}

func DeleteUserTotalCount(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserTotalCount,
		bson.M{"SUID": suid},
	)
}
