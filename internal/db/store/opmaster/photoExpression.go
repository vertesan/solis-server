package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/master/mname"
	"github.com/vertesan/solis-server/internal/db/def/master/mstruct"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllPhotoExpressions(ctx context.Context) ([]*pmaster.PhotoExpression, error) {
	return mcommon.FindMany[mstruct.PhotoExpression, pmaster.PhotoExpression](
		ctx, mname.PhotoExpression, bson.M{},
	)
}
