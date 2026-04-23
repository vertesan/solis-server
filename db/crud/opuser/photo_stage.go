package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserPhotoStages(ctx context.Context, suid string) ([]*papi.PhotoStage, error) {
	return ucommon.FindAll[papi.PhotoStage](
		ctx,
		uname.UserPhotoStage,
		bson.M{"SUID": suid},
	)
}

func UpsertUserPhotoStage(ctx context.Context, suid string, stage *papi.PhotoStage) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserPhotoStage,
		bson.M{"SUID": suid, "stageId": stage.StageId},
		&ustruct.UserStruct[*papi.PhotoStage]{
			SUID: suid,
			Data: stage,
		},
	)
}

func UpsertManyUserPhotoStages(ctx context.Context, suid string, stages []*papi.PhotoStage) error {
	for _, s := range stages {
		if err := UpsertUserPhotoStage(ctx, suid, s); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserPhotoStages(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPhotoStage,
		bson.M{"SUID": suid},
	)
}
