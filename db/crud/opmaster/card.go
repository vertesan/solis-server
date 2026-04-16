package opmaster

import (
	"context"
	"iter"
	"vertesan/solis-server/db/crud/opmaster/internal/mcommon"
	"vertesan/solis-server/db/def/master/mname"
	"vertesan/solis-server/db/def/master/mstruct"
	"vertesan/solis-server/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindCard(ctx context.Context, id string) (*pmaster.Card, error) {
	return mcommon.FindOne[mstruct.Card, pmaster.Card](
		ctx, mname.Card, bson.M{"Id": id},
	)
}

func FindAllCards(ctx context.Context) ([]*pmaster.Card, error) {
	return mcommon.FindMany[mstruct.Card, pmaster.Card](
		ctx, mname.Card, bson.M{},
	)
}

func YieldAllCards(ctx context.Context) iter.Seq[*pmaster.Card] {
	return func(yield func(*pmaster.Card) bool) {
		for c := range mcommon.YieldMany[mstruct.Card, pmaster.Card](ctx, mname.Card, bson.M{}) {
			if !yield(c) {
				return
			}
		}
	}
}

func FindCardParameter(ctx context.Context, id string, level int64) (*pmaster.CardParameter, error) {
	return mcommon.FindOne[mstruct.CardParameter, pmaster.CardParameter](
		ctx, mname.CardParameter, bson.M{"Id": id, "Level": level},
	)
}

func FindCardRarity(ctx context.Context, rarity int64) (*pmaster.CardRarity, error) {
	return mcommon.FindOne[mstruct.CardRarity, pmaster.CardRarity](
		ctx, mname.CardRarity, bson.M{"Rarity": rarity},
	)
}

func FindCardLevelReleases(ctx context.Context, id string, lteLevel int32) ([]*pmaster.CardLevelRelease, error) {
	return mcommon.FindMany[mstruct.CardLevelRelease, pmaster.CardLevelRelease](
		ctx, mname.CardLevelRelease, bson.D{
			{Key: "Id", Value: id},
			{Key: "Level", Value: bson.M{"$lte": lteLevel}},
		}, options.Find().SetSort(bson.M{"Level": 1}),
	)
}
