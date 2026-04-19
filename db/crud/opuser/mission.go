package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserMissions(ctx context.Context, suid string) ([]*ptransaction.UserMission, error) {
	return ucommon.FindAll[ptransaction.UserMission](
		ctx,
		uname.UserMission,
		bson.M{"SUID": suid},
	)
}
