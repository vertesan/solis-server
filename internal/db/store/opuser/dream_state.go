package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserDreamState(ctx context.Context, suid string) (*umodel.UserDreamState, error) {
	return ucommon.FindOne[umodel.UserDreamState](ctx, uname.UserDreamState, bson.M{"_id": suid})
}

func UpsertUserDreamState(ctx context.Context, state *umodel.UserDreamState) error {
	return ucommon.UpsertOne(ctx, uname.UserDreamState, bson.M{"_id": state.SUID}, state)
}
