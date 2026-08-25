package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserGodState(ctx context.Context, suid string) (*umodel.UserGodState, error) {
	return ucommon.FindOne[umodel.UserGodState](
		ctx,
		uname.UserGodState,
		bson.M{"SUID": suid},
	)
}

func UpsertUserGodState(ctx context.Context, state *umodel.UserGodState) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserGodState,
		bson.M{"SUID": state.SUID},
		state,
	)
}

func DeleteUserGodState(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserGodState, bson.M{"SUID": suid})
}
