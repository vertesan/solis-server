package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserActivityFanEventProgress(ctx context.Context, suid string) (*ptransaction.UserActivityFanEventProgress, error) {
	return ucommon.FindOne[ptransaction.UserActivityFanEventProgress](
		ctx,
		uname.UserActivityFanEventProgress,
		bson.M{"SUID": suid},
	)
}
