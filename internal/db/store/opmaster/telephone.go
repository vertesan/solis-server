package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindTelephone(ctx context.Context, id string) (*pmaster.Telephone, error) {
	return mcommon.FindOne[mmodel.Telephone, pmaster.Telephone](
		ctx, mname.Telephone, bson.M{"Id": id},
	)
}
