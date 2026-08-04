package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserNotiStoneShopItem(ctx context.Context, suid string) (*papi.NotiShopItem, error) {
	return ucommon.FindOne[papi.NotiShopItem](
		ctx,
		uname.UserNotiStoneShopItem,
		bson.M{"SUID": suid},
	)
}

func UpsertUserNotiStoneShopItem(ctx context.Context, suid string, item *papi.NotiShopItem) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserNotiStoneShopItem,
		bson.M{"SUID": suid},
		&umodel.UserStruct[*papi.NotiShopItem]{
			SUID: suid,
			Data: item,
		},
	)
}

func DeleteUserNotiStoneShopItem(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserNotiStoneShopItem,
		bson.M{"SUID": suid},
	)
}

func FindUserNotiNormalShopItem(ctx context.Context, suid string) (*papi.NotiShopItem, error) {
	return ucommon.FindOne[papi.NotiShopItem](
		ctx,
		uname.UserNotiNormalShopItem,
		bson.M{"SUID": suid},
	)
}

func UpsertUserNotiNormalShopItem(ctx context.Context, suid string, item *papi.NotiShopItem) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserNotiNormalShopItem,
		bson.M{"SUID": suid},
		&umodel.UserStruct[*papi.NotiShopItem]{
			SUID: suid,
			Data: item,
		},
	)
}

func DeleteUserNotiNormalShopItem(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserNotiNormalShopItem,
		bson.M{"SUID": suid},
	)
}

func FindUserNotiShowcaseShopItem(ctx context.Context, suid string) (*papi.NotiShopItem, error) {
	return ucommon.FindOne[papi.NotiShopItem](
		ctx,
		uname.UserNotiShowcaseShopItem,
		bson.M{"SUID": suid},
	)
}

func UpsertUserNotiShowcaseShopItem(ctx context.Context, suid string, item *papi.NotiShopItem) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserNotiShowcaseShopItem,
		bson.M{"SUID": suid},
		&umodel.UserStruct[*papi.NotiShopItem]{
			SUID: suid,
			Data: item,
		},
	)
}

func DeleteUserNotiShowcaseShopItem(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserNotiShowcaseShopItem,
		bson.M{"SUID": suid},
	)
}
