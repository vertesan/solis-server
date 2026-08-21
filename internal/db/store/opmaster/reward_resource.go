package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindDecoration(ctx context.Context, id string) (*pmaster.Decoration, error) {
	return mcommon.FindOne[mmodel.Decoration, pmaster.Decoration](ctx, mname.Decoration, bson.M{"Id": id})
}

func FindEmblem(ctx context.Context, id string) (*pmaster.Emblem, error) {
	return mcommon.FindOne[mmodel.Emblem, pmaster.Emblem](ctx, mname.Emblem, bson.M{"Id": id})
}
