package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindLoveLevels(ctx context.Context, loveID string) ([]*pmaster.LoveLevel, error) {
	return mcommon.FindMany[mmodel.LoveLevel, pmaster.LoveLevel](ctx, mname.LoveLevel, bson.M{"LoveId": loveID})
}

func FindLoveStoryEpisodes(ctx context.Context, loveID string) ([]*pmaster.LoveStoryEpisode, error) {
	all, err := mcommon.FindMany[mmodel.LoveStoryEpisode, pmaster.LoveStoryEpisode](ctx, mname.LoveStoryEpisode, bson.M{})
	if err != nil {
		return nil, err
	}
	var result []*pmaster.LoveStoryEpisode
	for _, episode := range all {
		if episode.GetLoveId() == loveID {
			result = append(result, episode)
		}
	}
	return result, nil
}
