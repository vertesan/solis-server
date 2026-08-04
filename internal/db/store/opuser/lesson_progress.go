package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

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
		&umodel.UserStruct[*papi.ActivityLessonProgress]{
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
