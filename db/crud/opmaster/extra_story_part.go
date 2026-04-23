package opmaster

import (
	"context"
	"vertesan/solis-server/db/crud/opmaster/internal/mcommon"
	"vertesan/solis-server/db/def/master/mname"
	"vertesan/solis-server/db/def/master/mstruct"
	"vertesan/solis-server/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindExtraStoryPartsByIds(ctx context.Context, ids []string) ([]*pmaster.ExtraStoryPart, error) {
	return mcommon.FindMany[mstruct.ExtraStoryPart, pmaster.ExtraStoryPart](
		ctx, mname.ExtraStoryPart, bson.M{"Id": bson.M{"$in": ids}},
	)
}
