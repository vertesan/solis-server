package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserExtraStoryPartInfos(ctx context.Context, suid string) ([]*papi.ExtraStoryPartInfo, error) {
	return ucommon.FindAll[papi.ExtraStoryPartInfo](
		ctx,
		uname.UserExtraStoryPartInfo,
		bson.M{"SUID": suid},
	)
}

func UpsertUserExtraStoryPartInfo(ctx context.Context, suid string, info *papi.ExtraStoryPartInfo) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserExtraStoryPartInfo,
		bson.M{"SUID": suid, "id": info.Id},
		&umodel.UserStruct[*papi.ExtraStoryPartInfo]{
			SUID: suid,
			Data: info,
		},
	)
}

func UpsertManyUserExtraStoryPartInfos(ctx context.Context, suid string, infos []*papi.ExtraStoryPartInfo) error {
	for _, info := range infos {
		if err := UpsertUserExtraStoryPartInfo(ctx, suid, info); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserExtraStoryPartInfos(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserExtraStoryPartInfo,
		bson.M{"SUID": suid},
	)
}
