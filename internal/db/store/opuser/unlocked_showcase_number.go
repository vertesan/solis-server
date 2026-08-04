package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserUnlockedShowcaseNumber(ctx context.Context, suid string) (*umodel.UserUnlockedShowcaseNumber, error) {
	return ucommon.FindOne[umodel.UserUnlockedShowcaseNumber](
		ctx,
		uname.UserUnlockedShowcaseNumber,
		bson.M{"SUID": suid},
	)
}

func UpsertUserUnlockedShowcaseNumber(ctx context.Context, doc *umodel.UserUnlockedShowcaseNumber) error {
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
