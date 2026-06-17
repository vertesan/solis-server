package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserMusics(ctx context.Context, suid string) ([]*ptransaction.UserMusic, error) {
	return ucommon.FindAll[ptransaction.UserMusic](
		ctx,
		uname.UserMusic,
		bson.M{"SUID": suid},
	)
}
