package opmaster

import (
	"context"
	"iter"
	"vertesan/solis-server/db/def/master/mname"
	"vertesan/solis-server/db/def/master/mstruct"
	"vertesan/solis-server/db/store/opmaster/internal/mcommon"
	"vertesan/solis-server/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindCostume(ctx context.Context, id string) (*pmaster.Costume, error) {
	return mcommon.FindOne[mstruct.Costume, pmaster.Costume](
		ctx, mname.Costume, bson.M{"Id": id},
	)
}

func YieldAllCostumes(ctx context.Context) iter.Seq[*pmaster.Costume] {
	return func(yield func(*pmaster.Costume) bool) {
		for c := range mcommon.YieldMany[mstruct.Costume, pmaster.Costume](ctx, mname.Costume, bson.M{}) {
			if !yield(c) {
				return
			}
		}
	}
}
