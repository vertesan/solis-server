package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserDecorations(ctx context.Context, suid string) ([]*ptransaction.UserDecoration, error) {
	return ucommon.FindAll[ptransaction.UserDecoration](
		ctx,
		uname.UserDecoration,
		bson.M{"SUID": suid},
	)
}
