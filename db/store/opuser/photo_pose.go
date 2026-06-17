package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

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
		&ustruct.UserStruct[*ptransaction.UserPhotoPose]{
			SUID: suid,
			Data: photoPose,
		},
	)
}

func UpsertManyUserPhotoPoses(ctx context.Context, suid string, photoPoses []*ptransaction.UserPhotoPose) error {
	for _, photoPose := range photoPoses {
		if err := UpsertUserPhotoPose(ctx, suid, photoPose); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserPhotoPoses(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPhotoPose,
		bson.M{"SUID": suid},
	)
}
