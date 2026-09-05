package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserHomeTalk(ctx context.Context, suid, id string) (*ptransaction.UserHomeTalk, error) {
	return ucommon.FindOne[ptransaction.UserHomeTalk](ctx, uname.UserHomeTalk, bson.M{"SUID": suid, "homeTalkId": id})
}

func UpsertUserHomeTalk(ctx context.Context, suid string, talk *ptransaction.UserHomeTalk) error {
	return ucommon.UpsertOne(ctx, uname.UserHomeTalk,
		bson.M{"SUID": suid, "homeTalkId": talk.GetHomeTalkId()},
		&umodel.UserStruct[*ptransaction.UserHomeTalk]{SUID: suid, Data: talk},
	)
}

func FindAllUserHomeTalks(ctx context.Context, suid string) ([]*ptransaction.UserHomeTalk, error) {
	return ucommon.FindAll[ptransaction.UserHomeTalk](
		ctx,
		uname.UserHomeTalk,
		bson.M{"SUID": suid},
	)
}
