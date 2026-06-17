package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserShowcaseToyShopIds(ctx context.Context, suid string) (*ustruct.UserShowcaseToyShopId, error) {
	return ucommon.FindOne[ustruct.UserShowcaseToyShopId](
		ctx,
		uname.UserShowcaseToyShopId,
		bson.M{"SUID": suid},
	)
}

func UpsertUserShowcaseToyShopId(ctx context.Context, doc *ustruct.UserShowcaseToyShopId) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserShowcaseToyShopId,
		bson.M{"SUID": doc.SUID},
		doc,
	)
}

func DeleteUserShowcaseToyShopId(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserShowcaseToyShopId,
		bson.M{"SUID": suid},
	)
}
