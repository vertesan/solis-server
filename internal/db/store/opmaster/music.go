package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllMusics(ctx context.Context) ([]*pmaster.Music, error) {
	return mcommon.FindMany[mmodel.Music, pmaster.Music](
		ctx, mname.Music, bson.M{},
	)
}

func FindMusic(ctx context.Context, id string) (*pmaster.Music, error) {
	return mcommon.FindOne[mmodel.Music, pmaster.Music](
		ctx, mname.Music, bson.M{"Id": id},
	)
}
