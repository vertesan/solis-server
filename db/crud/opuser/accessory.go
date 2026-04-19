package opuser

import (
	"context"
	"fmt"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/internal/depot"
	"vertesan/solis-server/proto/ptransaction"

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
		&ustruct.UserStruct[*ptransaction.UserAccessory]{
			SUID: suid,
			Data: accessory,
		},
	)
}

// Note: This function is preferred to be used inside a transaction if there are more than 1 objects in the slice
func UpsertManyUserAccessories(ctx context.Context, suid string, accessories []*ptransaction.UserAccessory) error {
	for _, accessory := range accessories {
		if err := UpsertUserAccessory(ctx, suid, accessory); err != nil {
			return err
		}
	}
	return nil
}

func IncsertUserAccessories(ctx context.Context, suid string, accs []*ptransaction.UserAccessory) error {
	for _, acc := range accs {
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
	}
	return nil
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
