package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserDutyPoints(ctx context.Context, suid string) ([]*ptransaction.UserDutyPoint, error) {
	return ucommon.FindAll[ptransaction.UserDutyPoint](
		ctx,
		uname.UserDutyPoint,
		bson.M{"SUID": suid},
	)
}
