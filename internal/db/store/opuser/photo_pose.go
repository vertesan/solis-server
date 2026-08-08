package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserPhotoPoses(ctx context.Context, suid string) ([]*ptransaction.UserPhotoPose, error) {
	return ucommon.FindAll[ptransaction.UserPhotoPose](
		ctx,
		uname.UserPhotoPose,
		bson.M{"SUID": suid},
	)
}

func UpsertUserPhotoPose(ctx context.Context, suid string, photoPose *ptransaction.UserPhotoPose) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserPhotoPose,
		bson.M{"SUID": suid, "photoPoseId": photoPose.PhotoPoseId},
		&umodel.UserStruct[*ptransaction.UserPhotoPose]{
			SUID: suid,
			Data: photoPose,
		},
	)
}

func UpsertManyUserPhotoPoses(ctx context.Context, suid string, photoPoses []*ptransaction.UserPhotoPose) error {
	return ucommon.ForEachAtomically(ctx, photoPoses, func(ctx context.Context, photoPose *ptransaction.UserPhotoPose) error {
		return UpsertUserPhotoPose(ctx, suid, photoPose)
	})
}

func DeleteAllUserPhotoPoses(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPhotoPose,
		bson.M{"SUID": suid},
	)
}
