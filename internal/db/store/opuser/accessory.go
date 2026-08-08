package opuser

import (
	"context"
	"fmt"
	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindUserAccessory(ctx context.Context, suid string, accessoryId string) (*ptransaction.UserAccessory, error) {
	return ucommon.FindOne[ptransaction.UserAccessory](
		ctx,
		uname.UserAccessory,
		bson.M{"SUID": suid, "accessoryId": accessoryId},
	)
}

func FindAllUserAccessories(ctx context.Context, suid string) ([]*ptransaction.UserAccessory, error) {
	return ucommon.FindAll[ptransaction.UserAccessory](
		ctx,
		uname.UserAccessory,
		bson.M{"SUID": suid},
		options.Find().SetSort(bson.D{{Key: "accessoryId", Value: 1}}),
	)
}

func UpsertUserAccessory(ctx context.Context, suid string, accessory *ptransaction.UserAccessory) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserAccessory,
		bson.M{
			"SUID":        suid,
			"accessoryId": accessory.AccessoryId,
		},
		&umodel.UserStruct[*ptransaction.UserAccessory]{
			SUID: suid,
			Data: accessory,
		},
	)
}

func UpsertManyUserAccessories(ctx context.Context, suid string, accessories []*ptransaction.UserAccessory) error {
	return ucommon.ForEachAtomically(ctx, accessories, func(ctx context.Context, accessory *ptransaction.UserAccessory) error {
		return UpsertUserAccessory(ctx, suid, accessory)
	})
}

func IncsertUserAccessories(ctx context.Context, suid string, accs []*ptransaction.UserAccessory) error {
	return ucommon.ForEachAtomically(ctx, accs, func(ctx context.Context, acc *ptransaction.UserAccessory) error {
		_, err := depot.User.Collection(uname.UserAccessory).UpdateOne(
			ctx,
			bson.M{
				"SUID":        suid,
				"accessoryId": acc.AccessoryId,
			},
			bson.M{"$inc": bson.M{
				"amount": acc.Amount,
			}},
			options.UpdateOne().SetUpsert(true),
		)
		if err != nil {
			return fmt.Errorf("failed incserting userAccessories: %w", err)
		}
		return nil
	})
}

func DeleteUserAccessory(ctx context.Context, suid string, accessoryId string) error {
	return ucommon.Delete(
		ctx,
		uname.UserAccessory,
		bson.M{"SUID": suid, "accessoryId": accessoryId},
	)
}

func DeleteAllUserAccessories(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserAccessory,
		bson.M{"SUID": suid},
	)
}
