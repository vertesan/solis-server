package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindUserItems(ctx context.Context, suid string, itemId string) ([]*ptransaction.UserItem, error) {
	return ucommon.FindAll[ptransaction.UserItem](
		ctx,
		uname.UserItem,
		bson.M{"SUID": suid, "itemId": itemId},
		options.Find().SetSort(bson.D{{Key: "expiredTime", Value: 1}}),
	)
}

func FindAllUserItems(ctx context.Context, suid string) ([]*ptransaction.UserItem, error) {
	return ucommon.FindAll[ptransaction.UserItem](
		ctx,
		uname.UserItem,
		bson.M{"SUID": suid},
		options.Find().SetSort(bson.D{{Key: "itemId", Value: 1}, {Key: "expiredTime", Value: 1}}),
	)
}

func UpsertUserItem(ctx context.Context, suid string, item *ptransaction.UserItem) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserItem,
		bson.M{
			"SUID":        suid,
			"itemId":      item.ItemId,
			"expiredTime": item.ExpiredTime,
		},
		&umodel.UserStruct[*ptransaction.UserItem]{
			SUID: suid,
			Data: item,
		},
	)
}

func UpsertManyUserItems(ctx context.Context, suid string, items []*ptransaction.UserItem) error {
	return ucommon.ForEachAtomically(ctx, items, func(ctx context.Context, item *ptransaction.UserItem) error {
		return UpsertUserItem(ctx, suid, item)
	})
}

func DeleteUserItem(ctx context.Context, suid string, itemId string, expiredTime int64) error {
	return ucommon.Delete(
		ctx,
		uname.UserItem,
		bson.M{"SUID": suid, "itemId": itemId, "expiredTime": expiredTime},
	)
}

func DeleteAllUserItems(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserItem,
		bson.M{"SUID": suid},
	)
}
