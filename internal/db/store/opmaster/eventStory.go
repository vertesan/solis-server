package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllEventStories(ctx context.Context) ([]*pmaster.EventStory, error) {
	return mcommon.FindMany[mmodel.EventStory, pmaster.EventStory](
		ctx, mname.EventStory, bson.M{},
	)
}

func FindEventStory(ctx context.Context, id string) (*pmaster.EventStory, error) {
	return mcommon.FindOne[mmodel.EventStory, pmaster.EventStory](
		ctx, mname.EventStory, bson.M{"Id": id},
	)
}
