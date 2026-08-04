package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserShopIds(ctx context.Context, suid string) (*umodel.UserShopId, error) {
	return ucommon.FindOne[umodel.UserShopId](
		ctx,
		uname.UserShopId,
		bson.M{"SUID": suid},
	)
}

func UpsertUserShopId(ctx context.Context, userShopId *umodel.UserShopId) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserShopId,
		bson.M{"SUID": userShopId.SUID},
		userShopId,
	)
}

func DeleteUserShopId(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserShopId,
		bson.M{"SUID": suid},
	)
}
