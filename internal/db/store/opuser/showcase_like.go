package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

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
		&umodel.UserStruct[*ptransaction.UserShowcaseLike]{
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
