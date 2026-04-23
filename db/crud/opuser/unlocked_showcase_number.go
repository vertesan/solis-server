package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserUnlockedShowcaseNumber(ctx context.Context, suid string) (*ustruct.UserUnlockedShowcaseNumber, error) {
	return ucommon.FindOne[ustruct.UserUnlockedShowcaseNumber](
		ctx,
		uname.UserUnlockedShowcaseNumber,
		bson.M{"SUID": suid},
	)
}

func UpsertUserUnlockedShowcaseNumber(ctx context.Context, doc *ustruct.UserUnlockedShowcaseNumber) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserUnlockedShowcaseNumber,
		bson.M{"SUID": doc.SUID},
		doc,
	)
}

func DeleteUserUnlockedShowcaseNumber(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserUnlockedShowcaseNumber,
		bson.M{"SUID": suid},
	)
}
