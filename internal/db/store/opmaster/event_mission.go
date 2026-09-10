package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindEventMissions(ctx context.Context, ids []string) ([]*pmaster.EventMission, error) {
	if len(ids) == 0 {
		return []*pmaster.EventMission{}, nil
	}
	return mcommon.FindMany[mmodel.EventMission, pmaster.EventMission](ctx, mname.EventMission, bson.M{"Id": bson.M{"$in": ids}})
}
