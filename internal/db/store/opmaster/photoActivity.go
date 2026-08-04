package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/master/mname"
	"github.com/vertesan/solis-server/internal/db/def/master/mstruct"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllPhotoActivities(ctx context.Context) ([]*pmaster.PhotoActivity, error) {
	return mcommon.FindMany[mstruct.PhotoActivity, pmaster.PhotoActivity](
		ctx, mname.PhotoActivity, bson.M{},
	)
}
