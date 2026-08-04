package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserHierarchy(ctx context.Context, suid string) (*ptransaction.UserHierarchy, error) {
	return ucommon.FindOne[ptransaction.UserHierarchy](
		ctx,
		uname.UserHierarchy,
		bson.M{"SUID": suid},
	)
}

func UpsertUserHierarchy(ctx context.Context, suid string, hierarchy *ptransaction.UserHierarchy) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserHierarchy,
		bson.M{"SUID": suid},
		&umodel.UserStruct[*ptransaction.UserHierarchy]{
			SUID: suid,
			Data: hierarchy,
		},
	)
}

func DeleteUserHierarchy(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserHierarchy,
		bson.M{"SUID": suid},
	)
}
