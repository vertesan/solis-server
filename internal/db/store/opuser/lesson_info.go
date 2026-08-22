package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserLessonInfo(ctx context.Context, suid string) (*papi.LessonInfo, error) {
	return ucommon.FindOne[papi.LessonInfo](ctx, uname.UserLessonInfo, bson.M{"SUID": suid})
}

func UpsertUserLessonInfo(ctx context.Context, suid string, info *papi.LessonInfo) error {
	stored := &papi.LessonInfo{
		CampaignEffects:       info.GetCampaignEffects(),
		CampaignDropRewardIds: info.GetCampaignDropRewardIds(),
	}
	return ucommon.UpsertOne(ctx, uname.UserLessonInfo, bson.M{"SUID": suid}, &umodel.UserStruct[*papi.LessonInfo]{
		SUID: suid,
		Data: stored,
	})
}

func DeleteUserLessonInfo(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserLessonInfo, bson.M{"SUID": suid})
}
