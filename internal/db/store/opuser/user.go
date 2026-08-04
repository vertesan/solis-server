package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUser(ctx context.Context, suid string) (*ptransaction.User, error) {
	return ucommon.FindOne[ptransaction.User](
		ctx,
		uname.User,
		bson.M{"SUID": suid},
	)
}

func UpsertUser(ctx context.Context, suid string, user *ptransaction.User) error {
	return ucommon.UpsertOne(
		ctx,
		uname.User,
		bson.M{"SUID": suid},
		&umodel.UserStruct[*ptransaction.User]{
			SUID: suid,
			Data: user,
		},
	)
}

func DeleteUser(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.User,
		bson.M{"SUID": suid},
	)
}
