package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserActivityFanEventProgress(ctx context.Context, suid string) (*ptransaction.UserActivityFanEventProgress, error) {
	return ucommon.FindOne[ptransaction.UserActivityFanEventProgress](
		ctx,
		uname.UserActivityFanEventProgress,
		bson.M{"SUID": suid},
	)
}
