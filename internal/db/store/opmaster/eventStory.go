package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/master/mname"
	"github.com/vertesan/solis-server/internal/db/def/master/mstruct"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllEventStories(ctx context.Context) ([]*pmaster.EventStory, error) {
	return mcommon.FindMany[mstruct.EventStory, pmaster.EventStory](
		ctx, mname.EventStory, bson.M{},
	)
}
