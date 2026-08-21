package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
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

func FindUserEmblem(ctx context.Context, suid, emblemID string) (*ptransaction.UserEmblem, error) {
	return ucommon.FindOne[ptransaction.UserEmblem](
		ctx,
		uname.UserEmblem,
		bson.M{"SUID": suid, "emblemId": emblemID},
	)
}

func UpsertUserEmblem(ctx context.Context, suid string, emblem *ptransaction.UserEmblem) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserEmblem,
		bson.M{"SUID": suid, "emblemId": emblem.EmblemId},
		&umodel.UserStruct[*ptransaction.UserEmblem]{
			SUID: suid,
			Data: emblem,
		},
	)
}

func UpsertManyUserEmblems(ctx context.Context, suid string, emblems []*ptransaction.UserEmblem) error {
	return ucommon.ForEachAtomically(ctx, emblems, func(ctx context.Context, emblem *ptransaction.UserEmblem) error {
		return UpsertUserEmblem(ctx, suid, emblem)
	})
}

func DeleteAllUserEmblems(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserEmblem,
		bson.M{"SUID": suid},
	)
}
