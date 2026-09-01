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

func FindUserShowcase(ctx context.Context, suid string, number int32) (*ptransaction.UserShowcase, error) {
	return ucommon.FindOne[ptransaction.UserShowcase](
		ctx,
		uname.UserShowcase,
		bson.M{"SUID": suid, "number": number},
	)
}

func FindAllUserShowcases(ctx context.Context, suid string) ([]*ptransaction.UserShowcase, error) {
	return ucommon.FindAll[ptransaction.UserShowcase](
		ctx,
		uname.UserShowcase,
		bson.M{"SUID": suid},
		options.Find().SetSort(bson.D{{Key: "number", Value: 1}}),
	)
}

func UpsertUserShowcase(ctx context.Context, suid string, showcase *ptransaction.UserShowcase) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserShowcase,
		bson.M{"SUID": suid, "number": showcase.Number},
		&umodel.UserStruct[*ptransaction.UserShowcase]{SUID: suid, Data: showcase},
	)
}

func UpsertManyUserShowcases(ctx context.Context, suid string, showcases []*ptransaction.UserShowcase) error {
	return ucommon.ForEachAtomically(ctx, showcases, func(ctx context.Context, showcase *ptransaction.UserShowcase) error {
		return UpsertUserShowcase(ctx, suid, showcase)
	})
}

func DeleteUserShowcase(ctx context.Context, suid string, number int32) error {
	return ucommon.Delete(ctx, uname.UserShowcase, bson.M{"SUID": suid, "number": number})
}

func DeleteAllUserShowcases(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserShowcase, bson.M{"SUID": suid})
}
