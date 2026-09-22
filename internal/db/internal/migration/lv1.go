package migration

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// server first startup database initialization
func level1Migration() error {
	data, err := loadLevel1Data()
	if err != nil {
		return fmt.Errorf("failed to load level 1 migration data: %w", err)
	}

	return transaction.WithTransaction(context.Background(), func(ctx context.Context) (any, error) {
		// serverProfile
		if err := opsolis.DeleteServerProfile(ctx); err != nil {
			return nil, err
		}
		if err := opsolis.InsertServerProfile(ctx, data.serverProfile); err != nil {
			return nil, err
		}

		// division
		if err := opsolis.DeleteAllDivisions(ctx); err != nil {
			return nil, err
		}
		if err := opsolis.UpsertDivision(ctx, data.division); err != nil {
			return nil, err
		}

		// campaignEffects (empty)

		// bannerInfo
		// menu banners
		if err := opsolis.UpsertMenuBanner(ctx, data.menuBanner); err != nil {
			return nil, err
		}

		// exchangeBoothIds

		// notiStoneShopItem

		// notiNormalShopItem

		// notiShowcaseShopItem

		// functionMaintenanceInfos

		// notifications
		if err := opsolis.UpsertNoticeCatalog(ctx, data.noticeCatalog); err != nil {
			return nil, err
		}

		// home events
		if err := opsolis.UpsertHomeEventInfo(ctx, data.homeEventInfo); err != nil {
			return nil, err
		}

		// installs the server-wide commerce catalogs introduced by the Exchange and Shop services
		if err := UpsertCommerceCatalogs(ctx); err != nil {
			return nil, err
		}

		// update migration flag
		if err := opsolis.UpsertMigration(ctx, &smodel.Migration{Migration: 1}); err != nil {
			return nil, err
		}
		return nil, nil
	})
}

// EnsureUserAuthIndexes creates the uniqueness constraints relied on by the
// authentication and account-management flows. Partial filters deliberately
// exclude the empty token values assigned to new accounts.
func ensureUserAuthIndexes(ctx context.Context) error {
	indexes := depot.User.Collection(uname.UserAuth).Indexes()
	models := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "publicUserId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "serverUserId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "firebaseId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "refreshTokens", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetName("refreshTokens_nonempty_unique").
				SetPartialFilterExpression(bson.M{
					"refreshTokens.0": bson.M{"$exists": true},
				}),
		},
		{
			Keys: bson.D{{Key: "oneTimeToken", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.M{
					"oneTimeToken": bson.M{"$gt": ""},
				}),
		},
	}

	if _, err := indexes.CreateMany(ctx, models); err != nil {
		return fmt.Errorf("failed to ensure userAuth indexes: %w", err)
	}
	return nil
}

func ensureUserIndexes(ctx context.Context) error {
	definitions := []struct {
		collection string
		keys       bson.D
	}{
		{uname.UserCard, bson.D{{Key: "SUID", Value: 1}, {Key: "cardId", Value: 1}}},
		{uname.UserCharacter, bson.D{{Key: "SUID", Value: 1}, {Key: "characterId", Value: 1}}},
		{uname.UserCostume, bson.D{{Key: "SUID", Value: 1}, {Key: "costumeId", Value: 1}}},
		{uname.UserHair, bson.D{{Key: "SUID", Value: 1}, {Key: "hairId", Value: 1}}},
		{uname.UserDecoration, bson.D{{Key: "SUID", Value: 1}, {Key: "decorationId", Value: 1}}},
		{uname.UserEmblem, bson.D{{Key: "SUID", Value: 1}, {Key: "emblemId", Value: 1}}},
		{uname.UserMusic, bson.D{{Key: "SUID", Value: 1}, {Key: "musicId", Value: 1}}},
		{uname.UserLove, bson.D{{Key: "SUID", Value: 1}, {Key: "loveId", Value: 1}}},
		{uname.UserCharacterMusic, bson.D{{Key: "SUID", Value: 1}, {Key: "characterId", Value: 1}, {Key: "musicId", Value: 1}}},
		{uname.UserHomePosition, bson.D{{Key: "SUID", Value: 1}, {Key: "homePositionType", Value: 1}}},
		{uname.UserHomeTalk, bson.D{{Key: "SUID", Value: 1}, {Key: "homeTalkId", Value: 1}}},
		{uname.UserStory, bson.D{{Key: "SUID", Value: 1}, {Key: "storyId", Value: 1}}},
		{uname.UserTelephone, bson.D{{Key: "SUID", Value: 1}, {Key: "telephoneId", Value: 1}}},
		{uname.UserMessage, bson.D{{Key: "SUID", Value: 1}, {Key: "messageId", Value: 1}}},
		{uname.UserMessageSchedule, bson.D{{Key: "SUID", Value: 1}, {Key: "messageId", Value: 1}}},
		{uname.UserShowcase, bson.D{{Key: "SUID", Value: 1}, {Key: "number", Value: 1}}},
		{uname.UserShowcaseLayout, bson.D{{Key: "SUID", Value: 1}, {Key: "number", Value: 1}}},
		{uname.UserShowcaseMusic, bson.D{{Key: "SUID", Value: 1}, {Key: "showcaseMusicId", Value: 1}}},
		{uname.UserShowcaseToy, bson.D{{Key: "SUID", Value: 1}, {Key: "showcaseToyId", Value: 1}}},
		{uname.UserShowcaseMyset, bson.D{{Key: "SUID", Value: 1}, {Key: "number", Value: 1}}},
		{uname.UserShowcaseMysetLayout, bson.D{{Key: "SUID", Value: 1}, {Key: "number", Value: 1}}},
		{uname.UserStaff, bson.D{{Key: "SUID", Value: 1}, {Key: "parameterType", Value: 1}}},
		{uname.UserArea, bson.D{{Key: "SUID", Value: 1}, {Key: "areaId", Value: 1}}},
		{uname.UserQuest, bson.D{{Key: "SUID", Value: 1}, {Key: "questId", Value: 1}}},
		{uname.UserDreamArea, bson.D{{Key: "SUID", Value: 1}, {Key: "basePhotoId", Value: 1}}},
		{uname.UserDreamState, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserQuestLiveHistory, bson.D{{Key: "SUID", Value: 1}, {Key: "questId", Value: 1}}},
		{uname.UserQuestLiveReplay, bson.D{{Key: "SUID", Value: 1}, {Key: "questId", Value: 1}, {Key: "_id", Value: 1}}},
		{uname.UserQuestLiveChunk, bson.D{{Key: "SUID", Value: 1}, {Key: "questId", Value: 1}, {Key: "replayId", Value: 1}, {Key: "number", Value: 1}}},
		{uname.UserTutorial, bson.D{{Key: "SUID", Value: 1}, {Key: "tutorialType", Value: 1}}},
		{uname.UserEventStoryInfo, bson.D{{Key: "SUID", Value: 1}, {Key: "id", Value: 1}}},
		{uname.UserExtraStoryPartInfo, bson.D{{Key: "SUID", Value: 1}, {Key: "id", Value: 1}}},
		{uname.UserPhotoActivity, bson.D{{Key: "SUID", Value: 1}, {Key: "photoActivityId", Value: 1}}},
		{uname.UserPhotoExpression, bson.D{{Key: "SUID", Value: 1}, {Key: "photoExpressionId", Value: 1}}},
		{uname.UserPhotoPose, bson.D{{Key: "SUID", Value: 1}, {Key: "photoPoseId", Value: 1}}},
		{uname.UserPhotoMusic, bson.D{{Key: "SUID", Value: 1}, {Key: "musicId", Value: 1}}},
		{uname.UserPhotoStage, bson.D{{Key: "SUID", Value: 1}, {Key: "stageId", Value: 1}}},
		{uname.UserAccessory, bson.D{{Key: "SUID", Value: 1}, {Key: "accessoryId", Value: 1}}},
		{uname.UserItem, bson.D{{Key: "SUID", Value: 1}, {Key: "itemId", Value: 1}, {Key: "expiredTime", Value: 1}}},
		{uname.UserPhoto, bson.D{{Key: "SUID", Value: 1}, {Key: "photoId", Value: 1}}},
		{uname.UserPhotoGift, bson.D{{Key: "SUID", Value: 1}, {Key: "photoId", Value: 1}}},
		{uname.UserPhotoRecipe, bson.D{{Key: "SUID", Value: 1}, {Key: "photoRecipeId", Value: 1}}},
		{uname.UserPoint, bson.D{{Key: "SUID", Value: 1}, {Key: "pointType", Value: 1}}},
		{uname.UserNotification, bson.D{{Key: "SUID", Value: 1}, {Key: "notificationType", Value: 1}}},
		{uname.UserMission, bson.D{{Key: "SUID", Value: 1}, {Key: "missionId", Value: 1}}},
		{uname.UserCardSupport, bson.D{{Key: "SUID", Value: 1}, {Key: "number", Value: 1}}},
		{uname.UserCardSupport, bson.D{{Key: "SUID", Value: 1}, {Key: "cardId", Value: 1}}},
		{uname.UserExchangePurchase, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserShopPurchase, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserGodState, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserGachaInfo, bson.D{{Key: "SUID", Value: 1}, {Key: "id", Value: 1}}},
		{uname.UserGift, bson.D{{Key: "SUID", Value: 1}, {Key: "giftId", Value: 1}}},
		{uname.UserGiftHistory, bson.D{{Key: "SUID", Value: 1}, {Key: "giftId", Value: 1}}},
		{uname.UserLoginBonus, bson.D{{Key: "SUID", Value: 1}, {Key: "loginBonusId", Value: 1}}},
		{uname.UserActivityFanEventProgress, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserFanEventInfo, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserPromotionInfo, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserRefreshInfo, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserLessonProgress, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserLessonInfo, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserFanEventRun, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserPromotionRun, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserRefreshRun, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserActivityLessonDailyState, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserFanEventRanking, bson.D{{Key: "SUID", Value: 1}, {Key: "activityFanEventId", Value: 1}}},
		{uname.UserDiaryState, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserMessageProgress, bson.D{{Key: "SUID", Value: 1}, {Key: "messageId", Value: 1}}},
		{uname.UserMarketState, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserExcursionState, bson.D{{Key: "SUID", Value: 1}}},
		{uname.UserExcursionGazeReaction, bson.D{
			{Key: "SUID", Value: 1},
			{Key: "characterId", Value: 1},
			{Key: "excursionPlaceId", Value: 1},
			{Key: "gazeReactionNumber", Value: 1},
		}},
		{uname.UserGachaHistory, bson.D{{Key: "SUID", Value: 1}, {Key: "gachaType", Value: 1}}},
	}

	for _, definition := range definitions {
		model := mongo.IndexModel{
			Keys:    definition.keys,
			Options: options.Index().SetUnique(true),
		}
		if _, err := depot.User.Collection(definition.collection).Indexes().CreateOne(ctx, model); err != nil {
			return fmt.Errorf("failed to ensure identity index for %q: %w", definition.collection, err)
		}
	}
	return nil
}
