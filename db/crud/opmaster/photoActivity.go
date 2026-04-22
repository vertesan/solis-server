package opmaster

import (
	"context"
	"vertesan/solis-server/db/crud/opmaster/internal/mcommon"
	"vertesan/solis-server/db/def/master/mname"
	"vertesan/solis-server/db/def/master/mstruct"
	"vertesan/solis-server/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllPhotoActivities(ctx context.Context) ([]*pmaster.PhotoActivity, error) {
	return mcommon.FindMany[mstruct.PhotoActivity, pmaster.PhotoActivity](
		ctx, mname.PhotoActivity, bson.M{},
	)
}
