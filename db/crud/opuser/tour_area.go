package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserTourAreas(ctx context.Context, suid string) ([]*ptransaction.UserTourArea, error) {
	return ucommon.FindAll[ptransaction.UserTourArea](
		ctx,
		uname.UserTourArea,
		bson.M{"SUID": suid},
	)
}
