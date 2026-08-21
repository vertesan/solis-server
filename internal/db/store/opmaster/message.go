package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindMessage(ctx context.Context, id string) (*pmaster.Message, error) {
	return mcommon.FindOne[mmodel.Message, pmaster.Message](
		ctx, mname.Message, bson.M{"Id": id},
	)
}

func FindAllMessageGroups(ctx context.Context) ([]*pmaster.MessageGroup, error) {
	return mcommon.FindMany[mmodel.MessageGroup, pmaster.MessageGroup](
		ctx, mname.MessageGroup, bson.M{},
	)
}
