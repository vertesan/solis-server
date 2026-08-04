package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/def/user/ustruct"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserExchangeBooth(ctx context.Context, suid string) (*ustruct.UserExchangeBooth, error) {
	return ucommon.FindOne[ustruct.UserExchangeBooth](
		ctx,
		uname.UserExchangeBooth,
		bson.M{"SUID": suid},
	)
}

func UpsertUserExchangeBooth(ctx context.Context, booth *ustruct.UserExchangeBooth) error {
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
