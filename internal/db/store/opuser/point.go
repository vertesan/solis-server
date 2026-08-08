package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

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
		&umodel.UserStruct[*ptransaction.UserPoint]{
			SUID: suid,
			Data: point,
		},
	)
}

func UpsertManyUserPoints(ctx context.Context, suid string, points []*ptransaction.UserPoint) error {
	return ucommon.ForEachAtomically(ctx, points, func(ctx context.Context, point *ptransaction.UserPoint) error {
		return UpsertUserPoint(ctx, suid, point)
	})
}

func DeleteAllUserPoints(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPoint,
		bson.M{"SUID": suid},
	)
}
