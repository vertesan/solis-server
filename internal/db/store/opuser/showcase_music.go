package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserShowcaseMusics(ctx context.Context, suid string) ([]*ptransaction.UserShowcaseMusic, error) {
	return ucommon.FindAll[ptransaction.UserShowcaseMusic](
		ctx,
		uname.UserShowcaseMusic,
		bson.M{"SUID": suid},
	)
}
