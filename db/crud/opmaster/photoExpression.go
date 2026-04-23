package opmaster

import (
	"context"
	"vertesan/solis-server/db/crud/opmaster/internal/mcommon"
	"vertesan/solis-server/db/def/master/mname"
	"vertesan/solis-server/db/def/master/mstruct"
	"vertesan/solis-server/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllPhotoExpressions(ctx context.Context) ([]*pmaster.PhotoExpression, error) {
	return mcommon.FindMany[mstruct.PhotoExpression, pmaster.PhotoExpression](
		ctx, mname.PhotoExpression, bson.M{},
	)
}
