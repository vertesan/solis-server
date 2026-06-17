package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

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
		&ustruct.UserStruct[*ptransaction.UserHomePosition]{
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
