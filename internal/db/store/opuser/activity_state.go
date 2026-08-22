package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserActivityLessonDailyState(ctx context.Context, suid string) (*umodel.UserActivityLessonDailyState, error) {
	return ucommon.FindOne[umodel.UserActivityLessonDailyState](
		ctx, uname.UserActivityLessonDailyState, bson.M{"SUID": suid},
	)
}

func UpsertUserActivityLessonDailyState(ctx context.Context, state *umodel.UserActivityLessonDailyState) error {
	return ucommon.UpsertOne(
		ctx, uname.UserActivityLessonDailyState, bson.M{"SUID": state.SUID}, state,
	)
}

func FindUserFanEventRanking(ctx context.Context, suid, activityFanEventID string) (*umodel.UserFanEventRanking, error) {
	return ucommon.FindOne[umodel.UserFanEventRanking](
		ctx,
		uname.UserFanEventRanking,
		bson.M{"SUID": suid, "activityFanEventId": activityFanEventID},
	)
}

func UpsertUserFanEventRanking(ctx context.Context, ranking *umodel.UserFanEventRanking) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserFanEventRanking,
		bson.M{"SUID": ranking.SUID, "activityFanEventId": ranking.ActivityFanEventId},
		ranking,
	)
}
