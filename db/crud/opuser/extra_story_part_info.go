package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/papi"

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
		&ustruct.UserStruct[*papi.ExtraStoryPartInfo]{
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
