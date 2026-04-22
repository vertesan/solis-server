package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserRefreshInfo(ctx context.Context, suid string) (*papi.RefreshInfo, error) {
	return ucommon.FindOne[papi.RefreshInfo](
		ctx,
		uname.UserRefreshInfo,
		bson.M{"SUID": suid},
	)
}

func UpsertUserRefreshInfo(ctx context.Context, suid string, info *papi.RefreshInfo) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserRefreshInfo,
		bson.M{"SUID": suid},
		&ustruct.UserStruct[*papi.RefreshInfo]{
			SUID: suid,
			Data: info,
		},
	)
}

func DeleteUserRefreshInfo(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserRefreshInfo,
		bson.M{"SUID": suid},
	)
}
