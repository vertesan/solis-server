package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserAreas(ctx context.Context, suid string) ([]*ptransaction.UserArea, error) {
	return ucommon.FindAll[ptransaction.UserArea](
		ctx,
		uname.UserArea,
		bson.M{"SUID": suid},
	)
}
