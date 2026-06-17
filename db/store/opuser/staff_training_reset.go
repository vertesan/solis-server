package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserStaffTrainingReset(ctx context.Context, suid string) (*ptransaction.UserStaffTrainingReset, error) {
	return ucommon.FindOne[ptransaction.UserStaffTrainingReset](
		ctx,
		uname.UserStaffTrainingReset,
		bson.M{"SUID": suid},
	)
}
