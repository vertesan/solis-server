package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/def/user/ustruct"
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
		&ustruct.UserStruct[*papi.PhotoExpression]{
			SUID: suid,
			Data: expr,
		},
	)
}

func UpsertManyUserPhotoExpressions(ctx context.Context, suid string, exprs []*papi.PhotoExpression) error {
	for _, e := range exprs {
		if err := UpsertUserPhotoExpression(ctx, suid, e); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserPhotoExpressions(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPhotoExpression,
		bson.M{"SUID": suid},
	)
}
