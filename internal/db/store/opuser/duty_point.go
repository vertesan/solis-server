package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserDutyPoints(ctx context.Context, suid string) ([]*ptransaction.UserDutyPoint, error) {
	return ucommon.FindAll[ptransaction.UserDutyPoint](
		ctx,
		uname.UserDutyPoint,
		bson.M{"SUID": suid},
	)
}
