package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserExchangeBooth(ctx context.Context, suid string) (*umodel.UserExchangeBooth, error) {
	return ucommon.FindOne[umodel.UserExchangeBooth](
		ctx,
		uname.UserExchangeBooth,
		bson.M{"SUID": suid},
	)
}

func UpsertUserExchangeBooth(ctx context.Context, booth *umodel.UserExchangeBooth) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserExchangeBooth,
		bson.M{"SUID": booth.SUID},
		booth,
	)
}

func DeleteUserExchangeBooth(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserExchangeBooth,
		bson.M{"SUID": suid},
	)
}
