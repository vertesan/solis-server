package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserCardSupports(ctx context.Context, suid string) ([]*ptransaction.UserCardSupport, error) {
	return ucommon.FindAll[ptransaction.UserCardSupport](
		ctx,
		uname.UserCardSupport,
		bson.M{"SUID": suid},
	)
}
