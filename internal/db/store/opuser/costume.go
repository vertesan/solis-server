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

func FindUserCostume(ctx context.Context, suid string, costumeId string) (*ptransaction.UserCostume, error) {
	return ucommon.FindOne[ptransaction.UserCostume](
		ctx,
		uname.UserCostume,
		bson.M{"SUID": suid, "costumeId": costumeId},
	)
}

func FindAllUserCostumes(ctx context.Context, suid string) ([]*ptransaction.UserCostume, error) {
	return ucommon.FindAll[ptransaction.UserCostume](
		ctx,
		uname.UserCostume,
		bson.M{"SUID": suid},
		options.Find().SetSort(bson.D{{Key: "costumeId", Value: 1}}),
	)
}

func UpsertUserCostume(ctx context.Context, suid string, costume *ptransaction.UserCostume) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserCostume,
		bson.M{
			"SUID":      suid,
			"costumeId": costume.CostumeId,
		},
		&umodel.UserStruct[*ptransaction.UserCostume]{
			SUID: suid,
			Data: costume,
		},
	)
}

// Note: This function is preferred to be used inside a transaction if there are more than 1 objects in the slice
func UpsertManyUserCostumes(ctx context.Context, suid string, costumes []*ptransaction.UserCostume) error {
	for _, costume := range costumes {
		if err := UpsertUserCostume(ctx, suid, costume); err != nil {
			return err
		}
	}
	return nil
}

func DeleteUserCostume(ctx context.Context, suid string, costumeId string) error {
	return ucommon.Delete(
		ctx,
		uname.UserCostume,
		bson.M{"SUID": suid, "costumeId": costumeId},
	)
}

func DeleteAllUserCostumes(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserCostume,
		bson.M{"SUID": suid},
	)
}
