package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserEventStoryInfo(ctx context.Context, suid string, id string) (*papi.EventStoryInfo, error) {
	return ucommon.FindOne[papi.EventStoryInfo](
		ctx,
		uname.UserEventStoryInfo,
		bson.M{"SUID": suid, "id": id},
	)
}

func FindAllUserEventStoryInfos(ctx context.Context, suid string) ([]*papi.EventStoryInfo, error) {
	return ucommon.FindAll[papi.EventStoryInfo](
		ctx,
		uname.UserEventStoryInfo,
		bson.M{"SUID": suid},
	)
}

func UpsertUserEventStoryInfo(ctx context.Context, suid string, info *papi.EventStoryInfo) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserEventStoryInfo,
		bson.M{"SUID": suid, "id": info.Id},
		&umodel.UserStruct[*papi.EventStoryInfo]{
			SUID: suid,
			Data: info,
		},
	)
}

func UpsertManyUserEventStoryInfos(ctx context.Context, suid string, infos []*papi.EventStoryInfo) error {
	for _, info := range infos {
		if err := UpsertUserEventStoryInfo(ctx, suid, info); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserEventStoryInfos(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserEventStoryInfo,
		bson.M{"SUID": suid},
	)
}
