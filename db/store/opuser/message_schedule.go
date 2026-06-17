package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserMessageSchedules(ctx context.Context, suid string) ([]*ptransaction.UserMessageSchedule, error) {
	return ucommon.FindAll[ptransaction.UserMessageSchedule](
		ctx,
		uname.UserMessageSchedule,
		bson.M{"SUID": suid},
	)
}
