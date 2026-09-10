package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserLoveState(ctx context.Context, suid, loveID string) (*umodel.UserLoveState, error) {
	return ucommon.FindOne[umodel.UserLoveState](ctx, uname.UserLoveState, bson.M{"SUID": suid, "loveId": loveID})
}

func UpsertUserLoveState(ctx context.Context, state *umodel.UserLoveState) error {
	return ucommon.UpsertOne(ctx, uname.UserLoveState, bson.M{"SUID": state.SUID, "loveId": state.LoveID}, state)
}
