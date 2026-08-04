package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserHierarchyNotiInfo(ctx context.Context, suid string) (*papi.HierarchyNotiInfo, error) {
	return ucommon.FindOne[papi.HierarchyNotiInfo](
		ctx,
		uname.UserHierarchyNotiInfo,
		bson.M{"SUID": suid},
	)
}

func UpsertUserHierarchyNotiInfo(ctx context.Context, suid string, info *papi.HierarchyNotiInfo) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserHierarchyNotiInfo,
		bson.M{"SUID": suid},
		&umodel.UserStruct[*papi.HierarchyNotiInfo]{
			SUID: suid,
			Data: info,
		},
	)
}

func DeleteUserHierarchyNotiInfo(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserHierarchyNotiInfo,
		bson.M{"SUID": suid},
	)
}
