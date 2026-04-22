package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/papi"

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
		&ustruct.UserStruct[*papi.NotiShopItem]{
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
		&ustruct.UserStruct[*papi.NotiShopItem]{
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
