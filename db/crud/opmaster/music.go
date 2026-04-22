package opmaster

import (
	"context"
	"vertesan/solis-server/db/crud/opmaster/internal/mcommon"
	"vertesan/solis-server/db/def/master/mname"
	"vertesan/solis-server/db/def/master/mstruct"
	"vertesan/solis-server/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllMusics(ctx context.Context) ([]*pmaster.Music, error) {
	return mcommon.FindMany[mstruct.Music, pmaster.Music](
		ctx, mname.Music, bson.M{},
	)
}
