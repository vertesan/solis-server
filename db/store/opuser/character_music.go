package opuser

import (
	"context"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/store/opuser/internal/ucommon"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserCharacterMusics(ctx context.Context, suid string) ([]*ptransaction.UserCharacterMusic, error) {
	return ucommon.FindAll[ptransaction.UserCharacterMusic](
		ctx,
		uname.UserCharacterMusic,
		bson.M{"SUID": suid},
	)
}
