package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/def/user/ustruct"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

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
