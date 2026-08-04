package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserStaffs(ctx context.Context, suid string) ([]*ptransaction.UserStaff, error) {
	return ucommon.FindAll[ptransaction.UserStaff](
		ctx,
		uname.UserStaff,
		bson.M{"SUID": suid},
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
	for _, staff := range staffs {
		if err := UpsertUserStaff(ctx, suid, staff); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserStaffs(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserStaff,
		bson.M{"SUID": suid},
	)
}
