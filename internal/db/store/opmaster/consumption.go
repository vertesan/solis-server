package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindConsumption(ctx context.Context, id string) (*pmaster.Consumption, error) {
	return mcommon.FindOne[mmodel.Consumption, pmaster.Consumption](
		ctx, mname.Consumption, bson.M{"Id": id},
	)
}

func FindReward(ctx context.Context, id string) (*pmaster.Reward, error) {
	return mcommon.FindOne[mmodel.Reward, pmaster.Reward](
		ctx, mname.Reward, bson.M{"Id": id},
	)
}
