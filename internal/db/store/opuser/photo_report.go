package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserPhotoReport(ctx context.Context, suid string) (*ptransaction.UserPhotoReport, error) {
	return ucommon.FindOne[ptransaction.UserPhotoReport](
		ctx,
		uname.UserPhotoReport,
		bson.M{"SUID": suid},
	)
}

func UpsertUserPhotoReport(ctx context.Context, suid string, photoReport *ptransaction.UserPhotoReport) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserPhotoReport,
		bson.M{"SUID": suid},
		&umodel.UserStruct[*ptransaction.UserPhotoReport]{
			SUID: suid,
			Data: photoReport,
		},
	)
}

func DeleteUserPhotoReport(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserPhotoReport,
		bson.M{"SUID": suid},
	)
}
