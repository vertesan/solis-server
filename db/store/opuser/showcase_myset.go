package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserShowcaseMysets(ctx context.Context, suid string) ([]*ptransaction.UserShowcaseMyset, error) {
	return ucommon.FindAll[ptransaction.UserShowcaseMyset](
		ctx,
		uname.UserShowcaseMyset,
		bson.M{"SUID": suid},
	)
}
