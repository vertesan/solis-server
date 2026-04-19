package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/ptransaction"

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
		&ustruct.UserStruct[*ptransaction.UserStaff]{
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
