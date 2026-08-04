package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserInvite(ctx context.Context, suid string) (*ptransaction.UserInvite, error) {
	return ucommon.FindOne[ptransaction.UserInvite](
		ctx,
		uname.UserInvite,
		bson.M{"SUID": suid},
	)
}
