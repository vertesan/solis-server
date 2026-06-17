package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserEventInfo(ctx context.Context, suid string) (*papi.HomeEventInfo, error) {
	return ucommon.FindOne[papi.HomeEventInfo](
		ctx,
		uname.UserEventInfo,
		bson.M{"SUID": suid},
	)
}

func UpsertUserEventInfo(ctx context.Context, suid string, info *papi.HomeEventInfo) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserEventInfo,
		bson.M{"SUID": suid},
		&ustruct.UserStruct[*papi.HomeEventInfo]{
			SUID: suid,
			Data: info,
		},
	)
}

func DeleteUserEventInfo(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserEventInfo,
		bson.M{"SUID": suid},
	)
}
