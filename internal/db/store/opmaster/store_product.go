package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindStoreProduct(ctx context.Context, productId string) (*pmaster.StoreProduct, error) {
	return mcommon.FindOne[mmodel.StoreProduct, pmaster.StoreProduct](
		ctx, mname.StoreProduct, bson.M{"ProductId": productId},
	)
}
