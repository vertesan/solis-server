package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserPhotoActivity(ctx context.Context, suid string, photoActivityId string) (*papi.PhotoActivity, error) {
	return ucommon.FindOne[papi.PhotoActivity](
		ctx,
		uname.UserPhotoActivity,
		bson.M{"SUID": suid, "photoActivityId": photoActivityId},
	)
}

func FindAllUserPhotoActivities(ctx context.Context, suid string) ([]*papi.PhotoActivity, error) {
	return ucommon.FindAll[papi.PhotoActivity](
		ctx,
		uname.UserPhotoActivity,
		bson.M{"SUID": suid},
	)
}

func UpsertUserPhotoActivity(ctx context.Context, suid string, activity *papi.PhotoActivity) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserPhotoActivity,
		bson.M{"SUID": suid, "photoActivityId": activity.PhotoActivityId},
		&umodel.UserStruct[*papi.PhotoActivity]{
			SUID: suid,
			Data: activity,
		},
	)
}

func UpsertManyUserPhotoActivities(ctx context.Context, suid string, activities []*papi.PhotoActivity) error {
	for _, a := range activities {
		if err := UpsertUserPhotoActivity(ctx, suid, a); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserPhotoActivities(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPhotoActivity,
		bson.M{"SUID": suid},
	)
}
