package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserFanEventInfo(ctx context.Context, suid string) (*papi.FanEventInfo, error) {
	return ucommon.FindOne[papi.FanEventInfo](
		ctx,
		uname.UserFanEventInfo,
		bson.M{"SUID": suid},
	)
}

func UpsertUserFanEventInfo(ctx context.Context, suid string, info *papi.FanEventInfo) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserFanEventInfo,
		bson.M{"SUID": suid},
		&ustruct.UserStruct[*papi.FanEventInfo]{
			SUID: suid,
			Data: info,
		},
	)
}

func DeleteUserFanEventInfo(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserFanEventInfo,
		bson.M{"SUID": suid},
	)
}
