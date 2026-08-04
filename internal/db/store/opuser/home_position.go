package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserHomePositions(ctx context.Context, suid string) ([]*ptransaction.UserHomePosition, error) {
	return ucommon.FindAll[ptransaction.UserHomePosition](
		ctx,
		uname.UserHomePosition,
		bson.M{"SUID": suid},
	)
}

func UpsertUserHomePosition(ctx context.Context, suid string, homePosition *ptransaction.UserHomePosition) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserHomePosition,
		bson.M{"SUID": suid, "homePositionType": homePosition.HomePositionType},
		&umodel.UserStruct[*ptransaction.UserHomePosition]{
			SUID: suid,
			Data: homePosition,
		},
	)
}

func UpsertManyUserHomePositions(ctx context.Context, suid string, homePositions []*ptransaction.UserHomePosition) error {
	for _, homePosition := range homePositions {
		if err := UpsertUserHomePosition(ctx, suid, homePosition); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserHomePositions(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserHomePosition,
		bson.M{"SUID": suid},
	)
}
