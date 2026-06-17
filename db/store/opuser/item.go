package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

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
		&ustruct.UserStruct[*ptransaction.UserItem]{
			SUID: suid,
			Data: item,
		},
	)
}

// Note: This function is preferred to be used inside a transaction if there are more than 1 objects in the slice
func UpsertManyUserItems(ctx context.Context, suid string, items []*ptransaction.UserItem) error {
	for _, item := range items {
		if err := UpsertUserItem(ctx, suid, item); err != nil {
			return err
		}
	}
	return nil
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
