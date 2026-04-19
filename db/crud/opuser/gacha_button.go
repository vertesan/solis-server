package opuser

import (
	"context"
	"vertesan/solis-server/db/crud/opuser/internal/ucommon"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserGachaButtons(ctx context.Context, suid string) ([]*ptransaction.UserGachaButton, error) {
	return ucommon.FindAll[ptransaction.UserGachaButton](
		ctx,
		uname.UserGachaButton,
		bson.M{"SUID": suid},
	)
}
