package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserDecorations(ctx context.Context, suid string) ([]*ptransaction.UserDecoration, error) {
	return ucommon.FindAll[ptransaction.UserDecoration](
		ctx,
		uname.UserDecoration,
		bson.M{"SUID": suid},
	)
}
