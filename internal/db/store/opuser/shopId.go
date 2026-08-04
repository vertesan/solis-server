package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/def/user/ustruct"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserShopIds(ctx context.Context, suid string) (*ustruct.UserShopId, error) {
	return ucommon.FindOne[ustruct.UserShopId](
		ctx,
		uname.UserShopId,
		bson.M{"SUID": suid},
	)
}

func UpsertUserShopId(ctx context.Context, userShopId *ustruct.UserShopId) error {
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
