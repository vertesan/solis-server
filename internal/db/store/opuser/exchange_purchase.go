package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserExchangePurchase(ctx context.Context, suid string) (*umodel.UserExchangePurchase, error) {
	return ucommon.FindOne[umodel.UserExchangePurchase](
		ctx,
		uname.UserExchangePurchase,
		bson.M{"SUID": suid},
	)
}

func UpsertUserExchangePurchase(ctx context.Context, purchase *umodel.UserExchangePurchase) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserExchangePurchase,
		bson.M{"SUID": purchase.SUID},
		purchase,
	)
}

func DeleteUserExchangePurchase(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserExchangePurchase, bson.M{"SUID": suid})
}
