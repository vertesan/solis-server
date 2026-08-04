package opmaster

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"iter"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindCard(ctx context.Context, id string) (*pmaster.Card, error) {
	return mcommon.FindOne[mmodel.Card, pmaster.Card](
		ctx, mname.Card, bson.M{"Id": id},
	)
}

func FindAllCards(ctx context.Context) ([]*pmaster.Card, error) {
	return mcommon.FindMany[mmodel.Card, pmaster.Card](
		ctx, mname.Card, bson.M{},
	)
}

func YieldAllCards(ctx context.Context) iter.Seq[*pmaster.Card] {
	return func(yield func(*pmaster.Card) bool) {
		for c := range mcommon.YieldMany[mmodel.Card, pmaster.Card](ctx, mname.Card, bson.M{}) {
			if !yield(c) {
				return
			}
		}
	}
}

func FindCardParameter(ctx context.Context, id string, level int64) (*pmaster.CardParameter, error) {
	return mcommon.FindOne[mmodel.CardParameter, pmaster.CardParameter](
		ctx, mname.CardParameter, bson.M{"Id": id, "Level": level},
	)
}

func FindCardRarity(ctx context.Context, rarity int64) (*pmaster.CardRarity, error) {
	return mcommon.FindOne[mmodel.CardRarity, pmaster.CardRarity](
		ctx, mname.CardRarity, bson.M{"Rarity": rarity},
	)
}

func FindCardLevelReleases(ctx context.Context, id string, lteLevel int32) ([]*pmaster.CardLevelRelease, error) {
	return mcommon.FindMany[mmodel.CardLevelRelease, pmaster.CardLevelRelease](
		ctx, mname.CardLevelRelease, bson.D{
			{Key: "Id", Value: id},
			{Key: "Level", Value: bson.M{"$lte": lteLevel}},
		}, options.Find().SetSort(bson.D{{Key: "Level", Value: 1}}),
	)
}
