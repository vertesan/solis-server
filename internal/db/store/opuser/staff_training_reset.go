package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
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

func UpsertUserStaffTrainingReset(
	ctx context.Context,
	suid string,
	reset *ptransaction.UserStaffTrainingReset,
) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserStaffTrainingReset,
		bson.M{"SUID": suid},
		&umodel.UserStruct[*ptransaction.UserStaffTrainingReset]{
			SUID: suid,
			Data: reset,
		},
	)
}
