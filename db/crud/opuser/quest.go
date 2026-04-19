package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserQuests(ctx context.Context, suid string) ([]*ptransaction.UserQuest, error) {
	return ucommon.FindAll[ptransaction.UserQuest](
		ctx,
		uname.UserQuest,
		bson.M{"SUID": suid},
	)
}
