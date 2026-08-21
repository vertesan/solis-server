package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindMissionsByIds(ctx context.Context, ids []string) ([]*pmaster.Mission, error) {
	if len(ids) == 0 {
		return []*pmaster.Mission{}, nil
	}
	return mcommon.FindMany[mmodel.Mission, pmaster.Mission](
		ctx,
		mname.Mission,
		bson.M{"Id": bson.M{"$in": ids}},
	)
}

func FindAllMissions(ctx context.Context) ([]*pmaster.Mission, error) {
	return mcommon.FindMany[mmodel.Mission, pmaster.Mission](
		ctx,
		mname.Mission,
		bson.M{},
	)
}
