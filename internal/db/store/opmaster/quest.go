package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindQuest(ctx context.Context, id string) (*pmaster.Quest, error) {
	return mcommon.FindOne[mmodel.Quest, pmaster.Quest](ctx, mname.Quest, bson.M{"Id": id})
}

func FindAllAreaGroups(ctx context.Context) ([]*pmaster.AreaGroup, error) {
	return mcommon.FindMany[mmodel.AreaGroup, pmaster.AreaGroup](ctx, mname.AreaGroup, bson.M{})
}

func FindAllQuestContestRankingSeasons(ctx context.Context) ([]*pmaster.QuestContestRankingSeason, error) {
	return mcommon.FindMany[mmodel.QuestContestRankingSeason, pmaster.QuestContestRankingSeason](ctx, mname.QuestContestRankingSeason, bson.M{})
}
