package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserLoves(ctx context.Context, suid string) ([]*ptransaction.UserLove, error) {
	return ucommon.FindAll[ptransaction.UserLove](
		ctx,
		uname.UserLove,
		bson.M{"SUID": suid},
	)
}
