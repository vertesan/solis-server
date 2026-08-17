package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindPhotoAllInOne(ctx context.Context, id string) (*pmaster.PhotoAllInOne, error) {
	return mcommon.FindOne[mmodel.PhotoAllInOne, pmaster.PhotoAllInOne](
		ctx, mname.PhotoAllInOne, bson.M{"Id": id},
	)
}
