package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindStory(ctx context.Context, id string) (*pmaster.Story, error) {
	return mcommon.FindOne[mmodel.Story, pmaster.Story](
		ctx, mname.Story, bson.M{"Id": id},
	)
}
