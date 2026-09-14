package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserQuest(ctx context.Context, suid, id string) (*ptransaction.UserQuest, error) {
	return ucommon.FindOne[ptransaction.UserQuest](ctx, uname.UserQuest, bson.M{"SUID": suid, "questId": id})
}

func UpsertUserQuest(ctx context.Context, suid string, quest *ptransaction.UserQuest) error {
	return ucommon.UpsertOne(ctx, uname.UserQuest, bson.M{"SUID": suid, "questId": quest.GetQuestId()},
		&umodel.UserStruct[*ptransaction.UserQuest]{SUID: suid, Data: quest})
}

func FindAllUserQuests(ctx context.Context, suid string) ([]*ptransaction.UserQuest, error) {
	return ucommon.FindAll[ptransaction.UserQuest](
		ctx,
		uname.UserQuest,
		bson.M{"SUID": suid},
	)
}
