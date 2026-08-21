package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/penum"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserGachaHistory(ctx context.Context, suid string, gachaType penum.GachaType) (*umodel.UserGachaHistory, error) {
	return ucommon.FindOne[umodel.UserGachaHistory](
		ctx,
		uname.UserGachaHistory,
		bson.M{"SUID": suid, "gachaType": gachaType},
	)
}

func UpsertUserGachaHistory(ctx context.Context, history *umodel.UserGachaHistory) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserGachaHistory,
		bson.M{"SUID": history.SUID, "gachaType": history.GachaType},
		history,
	)
}

func DeleteAllUserGachaHistories(ctx context.Context, suid string) error {
	return ucommon.Delete(ctx, uname.UserGachaHistory, bson.M{"SUID": suid})
}
