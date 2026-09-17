package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindQuestTicketItems(ctx context.Context, itemType penum.ItemType) ([]*pmaster.Item, error) {
	// Item indexes only Id in the master mirror; Type lives in the protobuf.
	items, err := FindAllItems(ctx)
	if err != nil {
		return nil, err
	}
	var matches []*pmaster.Item
	for _, item := range items {
		if item.GetType() == itemType {
			matches = append(matches, item)
		}
	}
	return matches, nil
}

func FindAllMusicMasteryLevels(ctx context.Context) ([]*pmaster.MusicMasteryLevel, error) {
	return mcommon.FindMany[mmodel.MusicMasteryLevel, pmaster.MusicMasteryLevel](ctx, mname.MusicMasteryLevel, bson.M{})
}

func FindMusicMasteryRewards(ctx context.Context, musicID string) ([]*pmaster.MusicMasteryReward, error) {
	return mcommon.FindMany[mmodel.MusicMasteryReward, pmaster.MusicMasteryReward](ctx, mname.MusicMasteryReward, bson.M{"MusicId": musicID})
}

func FindMusicMasteryRewardLevels(ctx context.Context, id string) ([]*pmaster.MusicMasteryRewardSet, error) {
	return mcommon.FindMany[mmodel.MusicMasteryRewardSet, pmaster.MusicMasteryRewardSet](ctx, mname.MusicMasteryRewardSet, bson.M{"Id": id})
}

func FindAllCardLevels(ctx context.Context) ([]*pmaster.CardLevel, error) {
	return mcommon.FindMany[mmodel.CardLevel, pmaster.CardLevel](ctx, mname.CardLevel, bson.M{})
}
