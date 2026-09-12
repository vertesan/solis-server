package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindStoryEpisodeConsumption(ctx context.Context, id string, releaseCount int32) (*pmaster.StoryEpisodeConsumption, error) {
	return mcommon.FindOne[mmodel.StoryEpisodeConsumption, pmaster.StoryEpisodeConsumption](ctx, mname.StoryEpisodeConsumption,
		bson.M{"Id": id, "ReleaseCount": int64(releaseCount)})
}
