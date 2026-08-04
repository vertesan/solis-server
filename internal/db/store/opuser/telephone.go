package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserTelephones(ctx context.Context, suid string) ([]*ptransaction.UserTelephone, error) {
	return ucommon.FindAll[ptransaction.UserTelephone](
		ctx,
		uname.UserTelephone,
		bson.M{"SUID": suid},
	)
}
