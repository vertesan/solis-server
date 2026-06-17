package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserTelephones(ctx context.Context, suid string) ([]*ptransaction.UserTelephone, error) {
	return ucommon.FindAll[ptransaction.UserTelephone](
		ctx,
		uname.UserTelephone,
		bson.M{"SUID": suid},
	)
}
