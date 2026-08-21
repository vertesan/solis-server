package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserDiaryState(ctx context.Context, suid string) (*umodel.UserDiaryState, error) {
	return ucommon.FindOne[umodel.UserDiaryState](
		ctx,
		uname.UserDiaryState,
		bson.M{"SUID": suid},
	)
}

func UpsertUserDiaryState(ctx context.Context, state *umodel.UserDiaryState) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserDiaryState,
		bson.M{"SUID": state.SUID},
		state,
	)
}
