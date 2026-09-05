package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindSkill(ctx context.Context, id string) (*pmaster.Skill, error) {
	return mcommon.FindOne[mmodel.Skill, pmaster.Skill](
		ctx, mname.Skill, bson.M{"Id": id},
	)
}

func FindLiveAbility(ctx context.Context, id string) (*pmaster.LiveAbility, error) {
	return mcommon.FindOne[mmodel.LiveAbility, pmaster.LiveAbility](
		ctx, mname.LiveAbility, bson.M{"Id": id},
	)
}

func FindActivityAbility(ctx context.Context, id string) (*pmaster.ActivityAbility, error) {
	return mcommon.FindOne[mmodel.ActivityAbility, pmaster.ActivityAbility](
		ctx, mname.ActivityAbility, bson.M{"Id": id},
	)
}

func FindCardRank(ctx context.Context, rank int32) (*pmaster.CardRank, error) {
	return mcommon.FindOne[mmodel.CardRank, pmaster.CardRank](
		ctx, mname.CardRank, bson.M{"Rank": rank},
	)
}
