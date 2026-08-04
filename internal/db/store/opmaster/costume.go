package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/master/mname"
	"github.com/vertesan/solis-server/internal/db/def/master/mstruct"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"iter"

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
