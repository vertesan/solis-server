package opmaster

import (
	"context"
	"vertesan/solis-server/db/crud/opmaster/internal/mcommon"
	"vertesan/solis-server/db/def/master/mname"
	"vertesan/solis-server/db/def/master/mstruct"
	"vertesan/solis-server/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindSetting(ctx context.Context, id int64) (*pmaster.Setting, error) {
	return mcommon.FindOne[mstruct.Setting, pmaster.Setting](
		ctx, mname.Setting, bson.M{"Id": id},
	)
}
