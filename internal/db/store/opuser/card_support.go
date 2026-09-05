package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserCardSupports(ctx context.Context, suid string) ([]*ptransaction.UserCardSupport, error) {
	return ucommon.FindAll[ptransaction.UserCardSupport](
		ctx,
		uname.UserCardSupport,
		bson.M{"SUID": suid},
	)
}

func UpsertUserCardSupport(ctx context.Context, suid string, support *ptransaction.UserCardSupport) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserCardSupport,
		bson.M{"SUID": suid, "number": support.Number},
		&umodel.UserStruct[*ptransaction.UserCardSupport]{
			SUID: suid,
			Data: support,
		},
	)
}

func DeleteUserCardSupport(ctx context.Context, suid string, number int32) error {
	return ucommon.Delete(ctx, uname.UserCardSupport, bson.M{"SUID": suid, "number": number})
}
