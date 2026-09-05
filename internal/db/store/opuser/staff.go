package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindUserStaff(ctx context.Context, suid string, parameterType penum.ParameterType) (*ptransaction.UserStaff, error) {
	return ucommon.FindOne[ptransaction.UserStaff](
		ctx,
		uname.UserStaff,
		bson.M{"SUID": suid, "parameterType": parameterType},
	)
}

func FindAllUserStaffs(ctx context.Context, suid string) ([]*ptransaction.UserStaff, error) {
	return ucommon.FindAll[ptransaction.UserStaff](
		ctx,
		uname.UserStaff,
		bson.M{"SUID": suid},
		options.Find().SetSort(bson.D{{Key: "parameterType", Value: 1}}),
	)
}

func UpsertUserStaff(ctx context.Context, suid string, staff *ptransaction.UserStaff) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserStaff,
		bson.M{"SUID": suid, "parameterType": staff.ParameterType},
		&umodel.UserStruct[*ptransaction.UserStaff]{
			SUID: suid,
			Data: staff,
		},
	)
}

func UpsertManyUserStaffs(ctx context.Context, suid string, staffs []*ptransaction.UserStaff) error {
	return ucommon.ForEachAtomically(ctx, staffs, func(ctx context.Context, staff *ptransaction.UserStaff) error {
		return UpsertUserStaff(ctx, suid, staff)
	})
}

func DeleteAllUserStaffs(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserStaff,
		bson.M{"SUID": suid},
	)
}
