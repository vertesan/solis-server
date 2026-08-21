package opmaster

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindGacha(ctx context.Context, id string) (*pmaster.Gacha, error) {
	return mcommon.FindOne[mmodel.Gacha, pmaster.Gacha](ctx, mname.Gacha, bson.M{"Id": id})
}

func FindAllGachas(ctx context.Context) ([]*pmaster.Gacha, error) {
	return mcommon.FindMany[mmodel.Gacha, pmaster.Gacha](
		ctx,
		mname.Gacha,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "Order", Value: 1}, {Key: "Id", Value: 1}}),
	)
}

func FindGachaButton(ctx context.Context, id string) (*pmaster.GachaButton, error) {
	return mcommon.FindOne[mmodel.GachaButton, pmaster.GachaButton](ctx, mname.GachaButton, bson.M{"Id": id})
}

func FindAllGachaButtons(ctx context.Context) ([]*pmaster.GachaButton, error) {
	return mcommon.FindMany[mmodel.GachaButton, pmaster.GachaButton](
		ctx,
		mname.GachaButton,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "Order", Value: 1}, {Key: "Id", Value: 1}}),
	)
}

func FindGachaExchange(ctx context.Context, id string) (*pmaster.GachaExchange, error) {
	return mcommon.FindOne[mmodel.GachaExchange, pmaster.GachaExchange](ctx, mname.GachaExchange, bson.M{"Id": id})
}

func FindAllGachaExchanges(ctx context.Context) ([]*pmaster.GachaExchange, error) {
	return mcommon.FindMany[mmodel.GachaExchange, pmaster.GachaExchange](
		ctx,
		mname.GachaExchange,
		// Some historical stamp rows use a numeric Id. They are not addressable
		// by the string IDs exposed by the Gacha API.
		bson.M{"Id": bson.M{"$type": "string"}},
		options.Find().SetSort(bson.D{{Key: "Order", Value: 1}, {Key: "Id", Value: 1}}),
	)
}

func FindCardDuplicateReward(ctx context.Context, initialRarity, currentRarity int32) (*pmaster.CardDuplicateReward, error) {
	return mcommon.FindOne[mmodel.CardDuplicateReward, pmaster.CardDuplicateReward](
		ctx,
		mname.CardDuplicateReward,
		bson.M{"InitialRarity": initialRarity, "CurrentRarity": currentRarity},
	)
}

func FindCardDuplicateExp(ctx context.Context, initialRarity int32) (*pmaster.CardDuplicateExp, error) {
	return mcommon.FindOne[mmodel.CardDuplicateExp, pmaster.CardDuplicateExp](
		ctx,
		mname.CardDuplicateExp,
		bson.M{"InitialRarity": initialRarity},
	)
}

func FindMaxCardRarity(ctx context.Context) (*pmaster.CardRarity, error) {
	return mcommon.FindOne[mmodel.CardRarity, pmaster.CardRarity](
		ctx,
		mname.CardRarity,
		bson.M{},
		options.FindOne().SetSort(bson.D{{Key: "Rarity", Value: -1}}),
	)
}

func FindAllCardRarities(ctx context.Context) ([]*pmaster.CardRarity, error) {
	return mcommon.FindMany[mmodel.CardRarity, pmaster.CardRarity](
		ctx,
		mname.CardRarity,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "Rarity", Value: 1}}),
	)
}

func FindCardEvolutionLevels(ctx context.Context, groupID string) ([]*pmaster.CardEvolutionLevel, error) {
	return mcommon.FindMany[mmodel.CardEvolutionLevel, pmaster.CardEvolutionLevel](
		ctx,
		mname.CardEvolutionLevel,
		bson.M{"GroupId": groupID},
		options.Find().SetSort(bson.D{{Key: "Level", Value: 1}}),
	)
}

func FindCardEvolutionLevelReleases(
	ctx context.Context,
	groupID string,
	lteLevel int32,
) ([]*pmaster.CardEvolutionLevelRelease, error) {
	return mcommon.FindMany[mmodel.CardEvolutionLevelRelease, pmaster.CardEvolutionLevelRelease](
		ctx,
		mname.CardEvolutionLevelRelease,
		bson.M{"GroupId": groupID, "Level": bson.M{"$lte": lteLevel}},
		options.Find().SetSort(bson.D{{Key: "Level", Value: 1}}),
	)
}

func FindCardEvolutions(
	ctx context.Context,
	cardID string,
	gtLevel int32,
	lteLevel int32,
) ([]*pmaster.CardEvolution, error) {
	return mcommon.FindMany[mmodel.CardEvolution, pmaster.CardEvolution](
		ctx,
		mname.CardEvolution,
		bson.M{
			"CardId":         cardID,
			"EvolutionLevel": bson.M{"$gt": gtLevel, "$lte": lteLevel},
		},
		options.Find().SetSort(bson.D{{Key: "EvolutionLevel", Value: 1}}),
	)
}
