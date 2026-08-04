package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/def/user/ustruct"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

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
