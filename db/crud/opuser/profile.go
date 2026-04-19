package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserProfile(ctx context.Context, suid string) (*ptransaction.UserProfile, error) {
	return ucommon.FindOne[ptransaction.UserProfile](
		ctx,
		uname.UserProfile,
		bson.M{"SUID": suid},
	)
}

func UpsertUserProfile(ctx context.Context, suid string, profile *ptransaction.UserProfile) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserProfile,
		bson.M{"SUID": suid},
		&ustruct.UserStruct[*ptransaction.UserProfile]{
			SUID: suid,
			Data: profile,
		},
	)
}

func DeleteUserProfile(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserProfile,
		bson.M{"SUID": suid},
	)
}
