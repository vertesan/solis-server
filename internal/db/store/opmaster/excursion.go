package opmaster

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/model/master/mmodel"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/internal/mcommon"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindExcursionPlace(ctx context.Context, id string) (*pmaster.ExcursionPlace, error) {
	return mcommon.FindOne[mmodel.ExcursionPlace, pmaster.ExcursionPlace](
		ctx, mname.ExcursionPlace, bson.M{"Id": id},
	)
}

func FindExcursionPlaceSetting(ctx context.Context, characterId, placeId string) (*pmaster.ExcursionPlaceSetting, error) {
	return mcommon.FindOne[mmodel.ExcursionPlaceSetting, pmaster.ExcursionPlaceSetting](
		ctx,
		mname.ExcursionPlaceSetting,
		bson.M{"CharacterId": characterId, "ExcursionPlaceId": placeId},
	)
}

func FindExcursionPlaceSettingsByCharacter(ctx context.Context, characterId string) ([]*pmaster.ExcursionPlaceSetting, error) {
	return mcommon.FindMany[mmodel.ExcursionPlaceSetting, pmaster.ExcursionPlaceSetting](
		ctx,
		mname.ExcursionPlaceSetting,
		bson.M{"CharacterId": characterId},
		options.Find().SetSort(bson.D{{Key: "ExcursionPlaceId", Value: 1}}),
	)
}

func FindExcursionGazeReaction(ctx context.Context, characterId, placeId string, number int32) (*pmaster.ExcursionGazeReaction, error) {
	return mcommon.FindOne[mmodel.ExcursionGazeReaction, pmaster.ExcursionGazeReaction](
		ctx,
		mname.ExcursionGazeReaction,
		bson.M{"CharacterId": characterId, "ExcursionPlaceId": placeId, "Number": number},
	)
}

func FindExcursionGazeReactions(ctx context.Context, characterId, placeId string) ([]*pmaster.ExcursionGazeReaction, error) {
	return mcommon.FindMany[mmodel.ExcursionGazeReaction, pmaster.ExcursionGazeReaction](
		ctx,
		mname.ExcursionGazeReaction,
		bson.M{
			"CharacterId":      characterId,
			"ExcursionPlaceId": placeId,
			"Number":           bson.M{"$gt": 0},
		},
		options.Find().SetSort(bson.D{{Key: "Number", Value: 1}}),
	)
}

func FindAllExcursionGazeReactions(ctx context.Context) ([]*pmaster.ExcursionGazeReaction, error) {
	return mcommon.FindMany[mmodel.ExcursionGazeReaction, pmaster.ExcursionGazeReaction](
		ctx,
		mname.ExcursionGazeReaction,
		bson.M{"Number": bson.M{"$gt": 0}},
	)
}

func FindExcursionGiftItem(ctx context.Context, characterId string) (*pmaster.Item, error) {
	settings, err := mcommon.FindMany[mmodel.CompanyEnjoyObtainPointItemSetting, pmaster.CompanyEnjoyObtainPointItemSetting](
		ctx,
		mname.CompanyEnjoyObtainPointItemSetting,
		bson.M{"CharacterId": characterId},
		options.Find().SetSort(bson.D{{Key: "ObtainPoint", Value: -1}, {Key: "ItemId", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	if len(settings) == 0 || settings[0].GetItemId() == "" || settings[0].GetObtainPoint() <= 0 {
		return nil, fmt.Errorf("character %q has no valid excursion gift setting", characterId)
	}
	best := settings[0]
	for _, setting := range settings[1:] {
		if setting.GetObtainPoint() != best.GetObtainPoint() {
			break
		}
		if setting.GetItemId() != best.GetItemId() {
			return nil, fmt.Errorf("character %q has ambiguous highest-value excursion gifts", characterId)
		}
	}
	return FindItem(ctx, best.GetItemId())
}
