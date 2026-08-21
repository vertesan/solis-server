package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllExtraStories(ctx context.Context) ([]*pmaster.ExtraStory, error) {
	return mcommon.FindMany[mmodel.ExtraStory, pmaster.ExtraStory](
		ctx, mname.ExtraStory, bson.M{},
	)
}

func FindExtraStory(ctx context.Context, id string) (*pmaster.ExtraStory, error) {
	return mcommon.FindOne[mmodel.ExtraStory, pmaster.ExtraStory](
		ctx, mname.ExtraStory, bson.M{"Id": id},
	)
}

func FindExtraStoriesByIds(ctx context.Context, ids []string) ([]*pmaster.ExtraStory, error) {
	return mcommon.FindMany[mmodel.ExtraStory, pmaster.ExtraStory](
		ctx, mname.ExtraStory, bson.M{"Id": bson.M{"$in": ids}},
	)
}
