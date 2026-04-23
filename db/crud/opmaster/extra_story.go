package opmaster

import (
	"context"
	"vertesan/solis-server/db/crud/opmaster/internal/mcommon"
	"vertesan/solis-server/db/def/master/mname"
	"vertesan/solis-server/db/def/master/mstruct"
	"vertesan/solis-server/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllExtraStories(ctx context.Context) ([]*pmaster.ExtraStory, error) {
	return mcommon.FindMany[mstruct.ExtraStory, pmaster.ExtraStory](
		ctx, mname.ExtraStory, bson.M{},
	)
}

func FindExtraStoriesByIds(ctx context.Context, ids []string) ([]*pmaster.ExtraStory, error) {
	return mcommon.FindMany[mstruct.ExtraStory, pmaster.ExtraStory](
		ctx, mname.ExtraStory, bson.M{"Id": bson.M{"$in": ids}},
	)
}
