package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/master/mname"
	"github.com/vertesan/solis-server/internal/db/def/master/mstruct"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindExtraStoryPartsByIds(ctx context.Context, ids []string) ([]*pmaster.ExtraStoryPart, error) {
	return mcommon.FindMany[mstruct.ExtraStoryPart, pmaster.ExtraStoryPart](
		ctx, mname.ExtraStoryPart, bson.M{"Id": bson.M{"$in": ids}},
	)
}
