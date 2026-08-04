package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserMissions(ctx context.Context, suid string) ([]*ptransaction.UserMission, error) {
	return ucommon.FindAll[ptransaction.UserMission](
		ctx,
		uname.UserMission,
		bson.M{"SUID": suid},
	)
}
