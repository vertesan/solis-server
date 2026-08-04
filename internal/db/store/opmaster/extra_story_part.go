package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindExtraStoryPartsByIds(ctx context.Context, ids []string) ([]*pmaster.ExtraStoryPart, error) {
	return mcommon.FindMany[mmodel.ExtraStoryPart, pmaster.ExtraStoryPart](
		ctx, mname.ExtraStoryPart, bson.M{"Id": bson.M{"$in": ids}},
	)
}
