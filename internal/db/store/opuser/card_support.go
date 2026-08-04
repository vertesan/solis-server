package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserCardSupports(ctx context.Context, suid string) ([]*ptransaction.UserCardSupport, error) {
	return ucommon.FindAll[ptransaction.UserCardSupport](
		ctx,
		uname.UserCardSupport,
		bson.M{"SUID": suid},
	)
}
