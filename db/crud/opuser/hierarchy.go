package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserHierarchy(ctx context.Context, suid string) (*ptransaction.UserHierarchy, error) {
	return ucommon.FindOne[ptransaction.UserHierarchy](
		ctx,
		uname.UserHierarchy,
		bson.M{"SUID": suid},
	)
}

func UpsertUserHierarchy(ctx context.Context, suid string, hierarchy *ptransaction.UserHierarchy) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserHierarchy,
		bson.M{"SUID": suid},
		&ustruct.UserStruct[*ptransaction.UserHierarchy]{
			SUID: suid,
			Data: hierarchy,
		},
	)
}

func DeleteUserHierarchy(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserHierarchy,
		bson.M{"SUID": suid},
	)
}
