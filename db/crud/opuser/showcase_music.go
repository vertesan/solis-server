package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserShowcaseMusics(ctx context.Context, suid string) ([]*ptransaction.UserShowcaseMusic, error) {
	return ucommon.FindAll[ptransaction.UserShowcaseMusic](
		ctx,
		uname.UserShowcaseMusic,
		bson.M{"SUID": suid},
	)
}
