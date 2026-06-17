package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserShowcases(ctx context.Context, suid string) ([]*ptransaction.UserShowcase, error) {
	return ucommon.FindAll[ptransaction.UserShowcase](
		ctx,
		uname.UserShowcase,
		bson.M{"SUID": suid},
	)
}
