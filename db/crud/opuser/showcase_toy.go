package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserShowcaseToys(ctx context.Context, suid string) ([]*ptransaction.UserShowcaseToy, error) {
	return ucommon.FindAll[ptransaction.UserShowcaseToy](
		ctx,
		uname.UserShowcaseToy,
		bson.M{"SUID": suid},
	)
}
