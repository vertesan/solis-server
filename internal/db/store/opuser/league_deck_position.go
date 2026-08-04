package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserLeagueDeckPositions(ctx context.Context, suid string) ([]*ptransaction.UserLeagueDeckPosition, error) {
	return ucommon.FindAll[ptransaction.UserLeagueDeckPosition](
		ctx,
		uname.UserLeagueDeckPosition,
		bson.M{"SUID": suid},
	)
}
