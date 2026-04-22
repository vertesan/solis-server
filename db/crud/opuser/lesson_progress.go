package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserLessonProgress(ctx context.Context, suid string) (*papi.ActivityLessonProgress, error) {
	return ucommon.FindOne[papi.ActivityLessonProgress](
		ctx,
		uname.UserLessonProgress,
		bson.M{"SUID": suid},
	)
}

func UpsertUserLessonProgress(ctx context.Context, suid string, progress *papi.ActivityLessonProgress) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserLessonProgress,
		bson.M{"SUID": suid},
		&ustruct.UserStruct[*papi.ActivityLessonProgress]{
			SUID: suid,
			Data: progress,
		},
	)
}

func DeleteUserLessonProgress(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserLessonProgress,
		bson.M{"SUID": suid},
	)
}
