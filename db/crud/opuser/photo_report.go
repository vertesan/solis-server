package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/ptransaction"

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
		&ustruct.UserStruct[*ptransaction.UserPhotoReport]{
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
