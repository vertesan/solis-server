package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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
		&umodel.UserStruct[*ptransaction.UserTotalCount]{
			SUID: suid,
			Data: totalCount,
		},
	)
}

func FindAndUpdateUserTotalCount(ctx context.Context, suid string, update map[string]any, opts ...options.Lister[options.FindOneAndUpdateOptions]) (*ptransaction.UserTotalCount, error) {
	return ucommon.FindAndUpdateOne[ptransaction.UserTotalCount](
		ctx,
		uname.UserTotalCount,
		bson.M{"SUID": suid},
		update,
		opts...,
	)
}

func DeleteUserTotalCount(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserTotalCount,
		bson.M{"SUID": suid},
	)
}
