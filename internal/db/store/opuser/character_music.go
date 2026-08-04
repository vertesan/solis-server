package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserCharacterMusics(ctx context.Context, suid string) ([]*ptransaction.UserCharacterMusic, error) {
	return ucommon.FindAll[ptransaction.UserCharacterMusic](
		ctx,
		uname.UserCharacterMusic,
		bson.M{"SUID": suid},
	)
}
