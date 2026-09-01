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

func FindUserShowcaseMyset(ctx context.Context, suid string, number int32) (*ptransaction.UserShowcaseMyset, error) {
	return ucommon.FindOne[ptransaction.UserShowcaseMyset](
		ctx,
		uname.UserShowcaseMyset,
		bson.M{"SUID": suid, "number": number},
	)
}

func FindAllUserShowcaseMysets(ctx context.Context, suid string) ([]*ptransaction.UserShowcaseMyset, error) {
	return ucommon.FindAll[ptransaction.UserShowcaseMyset](
		ctx,
		uname.UserShowcaseMyset,
		bson.M{"SUID": suid},
		options.Find().SetSort(bson.D{{Key: "number", Value: 1}}),
	)
}

func UpsertUserShowcaseMyset(ctx context.Context, suid string, myset *ptransaction.UserShowcaseMyset) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserShowcaseMyset,
		bson.M{"SUID": suid, "number": myset.Number},
		&umodel.UserStruct[*ptransaction.UserShowcaseMyset]{SUID: suid, Data: myset},
	)
}

func UpsertManyUserShowcaseMysets(ctx context.Context, suid string, mysets []*ptransaction.UserShowcaseMyset) error {
	return ucommon.ForEachAtomically(ctx, mysets, func(ctx context.Context, myset *ptransaction.UserShowcaseMyset) error {
		return UpsertUserShowcaseMyset(ctx, suid, myset)
	})
}

func DeleteUserShowcaseMyset(ctx context.Context, suid string, number int32) error {
	return ucommon.Delete(ctx, uname.UserShowcaseMyset, bson.M{"SUID": suid, "number": number})
}

func DeleteAllUserShowcaseMysets(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserShowcaseMyset, bson.M{"SUID": suid})
}
