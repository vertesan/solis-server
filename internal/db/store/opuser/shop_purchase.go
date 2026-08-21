package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserShopPurchase(ctx context.Context, suid string) (*umodel.UserShopPurchase, error) {
	return ucommon.FindOne[umodel.UserShopPurchase](
		ctx,
		uname.UserShopPurchase,
		bson.M{"SUID": suid},
	)
}

func UpsertUserShopPurchase(ctx context.Context, purchase *umodel.UserShopPurchase) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserShopPurchase,
		bson.M{"SUID": purchase.SUID},
		purchase,
	)
}

func DeleteUserShopPurchase(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserShopPurchase, bson.M{"SUID": suid})
}
