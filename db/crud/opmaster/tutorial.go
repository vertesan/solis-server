package opmaster

import (
	"context"
	"vertesan/solis-server/db/crud/opmaster/internal/mcommon"
	"vertesan/solis-server/db/def/master/mname"
	"vertesan/solis-server/db/def/master/mstruct"
	"vertesan/solis-server/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllTutorials(ctx context.Context) ([]*pmaster.Tutorial, error) {
	return mcommon.FindMany[mstruct.Tutorial, pmaster.Tutorial](
		ctx, mname.Tutorial, bson.M{},
	)
}
