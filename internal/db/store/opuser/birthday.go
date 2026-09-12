package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserBirthdayState(ctx context.Context, suid string) (*umodel.UserBirthdayState, error) {
	return ucommon.FindOne[umodel.UserBirthdayState](ctx, uname.UserBirthdayState, bson.M{"_id": suid})
}

func UpsertUserBirthdayState(ctx context.Context, state *umodel.UserBirthdayState) error {
	return ucommon.UpsertOne(ctx, uname.UserBirthdayState, bson.M{"_id": state.SUID}, state)
}
