package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/master/mname"
	"github.com/vertesan/solis-server/internal/db/def/master/mstruct"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllMusics(ctx context.Context) ([]*pmaster.Music, error) {
	return mcommon.FindMany[mstruct.Music, pmaster.Music](
		ctx, mname.Music, bson.M{},
	)
}
