package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserLeagueDeckPositions(ctx context.Context, suid string) ([]*ptransaction.UserLeagueDeckPosition, error) {
	return ucommon.FindAll[ptransaction.UserLeagueDeckPosition](
		ctx,
		uname.UserLeagueDeckPosition,
		bson.M{"SUID": suid},
	)
}
