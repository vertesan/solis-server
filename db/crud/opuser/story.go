package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserStories(ctx context.Context, suid string) ([]*ptransaction.UserStory, error) {
	return ucommon.FindAll[ptransaction.UserStory](
		ctx,
		uname.UserStory,
		bson.M{"SUID": suid},
	)
}
