package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/def/user/ustruct"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserEmblems(ctx context.Context, suid string) ([]*ptransaction.UserEmblem, error) {
	return ucommon.FindAll[ptransaction.UserEmblem](
		ctx,
		uname.UserEmblem,
		bson.M{"SUID": suid},
	)
}

func UpsertUserEmblem(ctx context.Context, suid string, emblem *ptransaction.UserEmblem) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserEmblem,
		bson.M{"SUID": suid, "emblemId": emblem.EmblemId},
		&ustruct.UserStruct[*ptransaction.UserEmblem]{
			SUID: suid,
			Data: emblem,
		},
	)
}

func UpsertManyUserEmblems(ctx context.Context, suid string, emblems []*ptransaction.UserEmblem) error {
	for _, emblem := range emblems {
		if err := UpsertUserEmblem(ctx, suid, emblem); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserEmblems(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserEmblem,
		bson.M{"SUID": suid},
	)
}
