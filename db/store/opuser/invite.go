package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserInvite(ctx context.Context, suid string) (*ptransaction.UserInvite, error) {
	return ucommon.FindOne[ptransaction.UserInvite](
		ctx,
		uname.UserInvite,
		bson.M{"SUID": suid},
	)
}
