package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserMarketState(ctx context.Context, suid string) (*umodel.UserMarketState, error) {
	return ucommon.FindOne[umodel.UserMarketState](ctx, uname.UserMarketState, bson.M{"SUID": suid})
}

func UpsertUserMarketState(ctx context.Context, state *umodel.UserMarketState) error {
	return ucommon.UpsertOne(ctx, uname.UserMarketState, bson.M{"SUID": state.SUID}, state)
}
