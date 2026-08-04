package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindSetting(ctx context.Context, id int64) (*pmaster.Setting, error) {
	return mcommon.FindOne[mmodel.Setting, pmaster.Setting](
		ctx, mname.Setting, bson.M{"Id": id},
	)
}
