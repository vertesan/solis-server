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

func FindMaxManagerLevel(ctx context.Context) (*pmaster.ManagerLevel, error) {
	return mcommon.FindOne[mmodel.ManagerLevel, pmaster.ManagerLevel](
		ctx,
		mname.ManagerLevel,
		bson.M{},
		options.FindOne().SetSort(bson.D{{Key: "Level", Value: -1}}),
	)
}

func FindMaxCardSupportRelease(ctx context.Context) (*pmaster.CardSupportRelease, error) {
	return mcommon.FindOne[mmodel.CardSupportRelease, pmaster.CardSupportRelease](
		ctx,
		mname.CardSupportRelease,
		bson.M{},
		options.FindOne().SetSort(bson.D{{Key: "Count", Value: -1}}),
	)
}

func FindMaxCharacterActivityLevel(ctx context.Context) (*pmaster.CharacterActivityLevel, error) {
	return mcommon.FindOne[mmodel.CharacterActivityLevel, pmaster.CharacterActivityLevel](
		ctx,
		mname.CharacterActivityLevel,
		bson.M{},
		options.FindOne().SetSort(bson.D{{Key: "Level", Value: -1}}),
	)
}

func FindAllAccessories(ctx context.Context) ([]*pmaster.Accessory, error) {
	return mcommon.FindMany[mmodel.Accessory, pmaster.Accessory](ctx, mname.Accessory, bson.M{})
}

func FindAllAreas(ctx context.Context) ([]*pmaster.Area, error) {
	return mcommon.FindMany[mmodel.Area, pmaster.Area](ctx, mname.Area, bson.M{})
}

func FindAllActivityFanEvents(ctx context.Context) ([]*pmaster.ActivityFanEvent, error) {
	return mcommon.FindMany[mmodel.ActivityFanEvent, pmaster.ActivityFanEvent](
		ctx,
		mname.ActivityFanEvent,
		bson.M{},
	)
}

func FindAllActivityPromotions(ctx context.Context) ([]*pmaster.ActivityPromotion, error) {
	return mcommon.FindMany[mmodel.ActivityPromotion, pmaster.ActivityPromotion](
		ctx,
		mname.ActivityPromotion,
		bson.M{},
	)
}

func FindAllActivityRefreshes(ctx context.Context) ([]*pmaster.ActivityRefresh, error) {
	return mcommon.FindMany[mmodel.ActivityRefresh, pmaster.ActivityRefresh](
		ctx,
		mname.ActivityRefresh,
		bson.M{},
	)
}

func FindAllCostumes(ctx context.Context) ([]*pmaster.Costume, error) {
	return mcommon.FindMany[mmodel.Costume, pmaster.Costume](ctx, mname.Costume, bson.M{})
}

func FindAllDecorations(ctx context.Context) ([]*pmaster.Decoration, error) {
	return mcommon.FindMany[mmodel.Decoration, pmaster.Decoration](ctx, mname.Decoration, bson.M{})
}

func FindAllEmblems(ctx context.Context) ([]*pmaster.Emblem, error) {
	return mcommon.FindMany[mmodel.Emblem, pmaster.Emblem](ctx, mname.Emblem, bson.M{})
}

func FindAllHairs(ctx context.Context) ([]*pmaster.Hair, error) {
	return mcommon.FindMany[mmodel.Hair, pmaster.Hair](ctx, mname.Hair, bson.M{})
}

func FindAllHomeTalks(ctx context.Context) ([]*pmaster.HomeTalk, error) {
	return mcommon.FindMany[mmodel.HomeTalk, pmaster.HomeTalk](ctx, mname.HomeTalk, bson.M{})
}

func FindAllItems(ctx context.Context) ([]*pmaster.Item, error) {
	return mcommon.FindMany[mmodel.Item, pmaster.Item](ctx, mname.Item, bson.M{})
}

func FindAllMessages(ctx context.Context) ([]*pmaster.Message, error) {
	return mcommon.FindMany[mmodel.Message, pmaster.Message](ctx, mname.Message, bson.M{})
}

func FindMaxLoveLevels(ctx context.Context) ([]*pmaster.LoveLevel, error) {
	levels, err := mcommon.FindMany[mmodel.LoveLevel, pmaster.LoveLevel](
		ctx,
		mname.LoveLevel,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "LoveId", Value: 1}, {Key: "Level", Value: -1}}),
	)
	if err != nil {
		return nil, err
	}
	result := make([]*pmaster.LoveLevel, 0)
	seen := make(map[string]struct{})
	for _, level := range levels {
		if _, exists := seen[level.LoveId]; exists {
			continue
		}
		seen[level.LoveId] = struct{}{}
		result = append(result, level)
	}
	return result, nil
}

func FindAllLoveHomeActions(ctx context.Context) ([]*pmaster.LoveHomeAction, error) {
	return mcommon.FindMany[mmodel.LoveHomeAction, pmaster.LoveHomeAction](
		ctx,
		mname.LoveHomeAction,
		bson.M{},
	)
}

func FindAllPhotoPoses(ctx context.Context) ([]*pmaster.PhotoPose, error) {
	return mcommon.FindMany[mmodel.PhotoPose, pmaster.PhotoPose](ctx, mname.PhotoPose, bson.M{})
}

func FindAllPhotoRecipes(ctx context.Context) ([]*pmaster.PhotoRecipe, error) {
	return mcommon.FindMany[mmodel.PhotoRecipe, pmaster.PhotoRecipe](ctx, mname.PhotoRecipe, bson.M{})
}

func FindAllQuests(ctx context.Context) ([]*pmaster.Quest, error) {
	return mcommon.FindMany[mmodel.Quest, pmaster.Quest](ctx, mname.Quest, bson.M{})
}

func FindAllStages(ctx context.Context) ([]*pmaster.Stage, error) {
	return mcommon.FindMany[mmodel.Stage, pmaster.Stage](ctx, mname.Stage, bson.M{})
}

func FindAllStories(ctx context.Context) ([]*pmaster.Story, error) {
	return mcommon.FindMany[mmodel.Story, pmaster.Story](ctx, mname.Story, bson.M{})
}

func FindAllStoryParts(ctx context.Context) ([]*pmaster.StoryPart, error) {
	return mcommon.FindMany[mmodel.StoryPart, pmaster.StoryPart](ctx, mname.StoryPart, bson.M{})
}

func FindMaxStaffLevels(ctx context.Context) ([]*pmaster.StaffLevel, error) {
	levels, err := mcommon.FindMany[mmodel.StaffLevel, pmaster.StaffLevel](
		ctx,
		mname.StaffLevel,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "ParameterType", Value: 1}, {Key: "Level", Value: -1}}),
	)
	if err != nil {
		return nil, err
	}
	return firstStaffLevelByParameter(levels), nil
}

func FindMaxStaffLimitBreakRanks(ctx context.Context) ([]*pmaster.StaffLevelLimitBreakRank, error) {
	ranks, err := mcommon.FindMany[mmodel.StaffLevelLimitBreakRank, pmaster.StaffLevelLimitBreakRank](
		ctx,
		mname.StaffLevelLimitBreakRank,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "ParameterType", Value: 1}, {Key: "Rank", Value: -1}}),
	)
	if err != nil {
		return nil, err
	}
	return firstStaffRankByParameter(ranks), nil
}

func firstStaffLevelByParameter(values []*pmaster.StaffLevel) []*pmaster.StaffLevel {
	result := make([]*pmaster.StaffLevel, 0)
	seen := make(map[int32]struct{})
	for _, value := range values {
		key := int32(value.ParameterType)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func firstStaffRankByParameter(values []*pmaster.StaffLevelLimitBreakRank) []*pmaster.StaffLevelLimitBreakRank {
	result := make([]*pmaster.StaffLevelLimitBreakRank, 0)
	seen := make(map[int32]struct{})
	for _, value := range values {
		key := int32(value.ParameterType)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func FindAllShowcaseMusics(ctx context.Context) ([]*pmaster.ShowcaseMusic, error) {
	return mcommon.FindMany[mmodel.ShowcaseMusic, pmaster.ShowcaseMusic](
		ctx, mname.ShowcaseMusic, bson.M{},
	)
}

func FindAllShowcaseToys(ctx context.Context) ([]*pmaster.ShowcaseToy, error) {
	return mcommon.FindMany[mmodel.ShowcaseToy, pmaster.ShowcaseToy](
		ctx, mname.ShowcaseToy, bson.M{},
	)
}

func FindAllTelephones(ctx context.Context) ([]*pmaster.Telephone, error) {
	return mcommon.FindMany[mmodel.Telephone, pmaster.Telephone](ctx, mname.Telephone, bson.M{})
}

func FindAllExtraStoryParts(ctx context.Context) ([]*pmaster.ExtraStoryPart, error) {
	return mcommon.FindMany[mmodel.ExtraStoryPart, pmaster.ExtraStoryPart](
		ctx, mname.ExtraStoryPart, bson.M{},
	)
}
