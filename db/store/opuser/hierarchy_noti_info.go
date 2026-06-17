package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/papi"

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
		&ustruct.UserStruct[*papi.HierarchyNotiInfo]{
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
