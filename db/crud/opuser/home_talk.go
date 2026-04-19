package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserHomeTalks(ctx context.Context, suid string) ([]*ptransaction.UserHomeTalk, error) {
	return ucommon.FindAll[ptransaction.UserHomeTalk](
		ctx,
		uname.UserHomeTalk,
		bson.M{"SUID": suid},
	)
}
