package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserPhotoExpressions(ctx context.Context, suid string) ([]*papi.PhotoExpression, error) {
	return ucommon.FindAll[papi.PhotoExpression](
		ctx,
		uname.UserPhotoExpression,
		bson.M{"SUID": suid},
	)
}

func UpsertUserPhotoExpression(ctx context.Context, suid string, expr *papi.PhotoExpression) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserPhotoExpression,
		bson.M{"SUID": suid, "photoExpressionId": expr.PhotoExpressionId},
		&umodel.UserStruct[*papi.PhotoExpression]{
			SUID: suid,
			Data: expr,
		},
	)
}

func UpsertManyUserPhotoExpressions(ctx context.Context, suid string, exprs []*papi.PhotoExpression) error {
	return ucommon.ForEachAtomically(ctx, exprs, func(ctx context.Context, expr *papi.PhotoExpression) error {
		return UpsertUserPhotoExpression(ctx, suid, expr)
	})
}

func DeleteAllUserPhotoExpressions(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPhotoExpression,
		bson.M{"SUID": suid},
	)
}
