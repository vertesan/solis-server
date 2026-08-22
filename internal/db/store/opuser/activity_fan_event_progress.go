package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
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

func UpsertUserActivityFanEventProgress(ctx context.Context, suid string, progress *ptransaction.UserActivityFanEventProgress) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserActivityFanEventProgress,
		bson.M{"SUID": suid},
		&umodel.UserStruct[*ptransaction.UserActivityFanEventProgress]{SUID: suid, Data: progress},
	)
}
