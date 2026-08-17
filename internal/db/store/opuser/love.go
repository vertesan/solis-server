package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserLoves(ctx context.Context, suid string) ([]*ptransaction.UserLove, error) {
	return ucommon.FindAll[ptransaction.UserLove](
		ctx,
		uname.UserLove,
		bson.M{"SUID": suid},
	)
}

func UpsertUserLove(ctx context.Context, suid string, love *ptransaction.UserLove) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserLove,
		bson.M{"SUID": suid, "loveId": love.LoveId},
		&umodel.UserStruct[*ptransaction.UserLove]{SUID: suid, Data: love},
	)
}
