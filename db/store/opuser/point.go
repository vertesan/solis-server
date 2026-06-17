package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserPoints(ctx context.Context, suid string) ([]*ptransaction.UserPoint, error) {
	return ucommon.FindAll[ptransaction.UserPoint](
		ctx,
		uname.UserPoint,
		bson.M{"SUID": suid},
	)
}

func UpsertUserPoint(ctx context.Context, suid string, point *ptransaction.UserPoint) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserPoint,
		bson.M{"SUID": suid, "pointType": point.PointType},
		&ustruct.UserStruct[*ptransaction.UserPoint]{
			SUID: suid,
			Data: point,
		},
	)
}

func UpsertManyUserPoints(ctx context.Context, suid string, points []*ptransaction.UserPoint) error {
	for _, point := range points {
		if err := UpsertUserPoint(ctx, suid, point); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserPoints(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPoint,
		bson.M{"SUID": suid},
	)
}
