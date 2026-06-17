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

func FindUserDeckPosition(ctx context.Context, suid string, number int32, position int32) ([]*ptransaction.UserDeckPosition, error) {
	return ucommon.FindAll[ptransaction.UserDeckPosition](
		ctx,
		uname.UserDeckPosition,
		bson.M{"SUID": suid, "number": number, "position": position},
	)
}

func FindUserDeckPositions(ctx context.Context, suid string, number int32) ([]*ptransaction.UserDeckPosition, error) {
	return ucommon.FindAll[ptransaction.UserDeckPosition](
		ctx,
		uname.UserDeckPosition,
		bson.M{"SUID": suid, "number": number},
	)
}

func FindAllUserDeckPositions(ctx context.Context, suid string) ([]*ptransaction.UserDeckPosition, error) {
	return ucommon.FindAll[ptransaction.UserDeckPosition](
		ctx,
		uname.UserDeckPosition,
		bson.M{"SUID": suid},
		options.Find().SetSort(bson.D{{Key: "number", Value: 1}, {Key: "position", Value: 1}}),
	)
}

func UpsertUserDeckPosition(ctx context.Context, suid string, pos *ptransaction.UserDeckPosition) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserDeckPosition,
		bson.M{
			"SUID":     suid,
			"number":   pos.Number,
			"position": pos.Position,
		},
		&ustruct.UserStruct[*ptransaction.UserDeckPosition]{
			SUID: suid,
			Data: pos,
		},
	)
}

// Note: This function is preferred to be used inside a transaction if there are more than 1 objects in the slice
func UpsertManyUserDeckPositions(ctx context.Context, suid string, positions []*ptransaction.UserDeckPosition) error {
	for _, pos := range positions {
		if err := UpsertUserDeckPosition(ctx, suid, pos); err != nil {
			return err
		}
	}
	return nil
}

func DeleteUserDeckPosition(ctx context.Context, suid string, number int32, position int32) error {
	return ucommon.Delete(
		ctx,
		uname.UserDeckPosition,
		bson.M{"SUID": suid, "number": number, "position": position},
	)
}

func DeleteUserDeckPositions(ctx context.Context, suid string, number int32) error {
	return ucommon.Delete(
		ctx,
		uname.UserDeckPosition,
		bson.M{"SUID": suid, "number": number},
	)
}

func DeleteAllUserDeckPositions(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserDeckPosition,
		bson.M{"SUID": suid},
	)
}
