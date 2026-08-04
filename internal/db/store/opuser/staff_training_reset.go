package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserStaffTrainingReset(ctx context.Context, suid string) (*ptransaction.UserStaffTrainingReset, error) {
	return ucommon.FindOne[ptransaction.UserStaffTrainingReset](
		ctx,
		uname.UserStaffTrainingReset,
		bson.M{"SUID": suid},
	)
}
