package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindCardSupportRelease(ctx context.Context, count int32) (*pmaster.CardSupportRelease, error) {
	return mcommon.FindOne[mmodel.CardSupportRelease, pmaster.CardSupportRelease](ctx, mname.CardSupportRelease, bson.M{"Count": int64(count)})
}
