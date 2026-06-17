package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserDuties(ctx context.Context, suid string) ([]*ptransaction.UserDuty, error) {
	return ucommon.FindAll[ptransaction.UserDuty](
		ctx,
		uname.UserDuty,
		bson.M{"SUID": suid},
	)
}
