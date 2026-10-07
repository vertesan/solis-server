package opuser

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type collection struct {
	Name      string
	SUIDField string
}

const (
	suidField         = "SUID"
	serverUserIDField = "serverUserId"
)

// DeleteAllUserData removes every document owned by suid from the registered
// user collections, relying on the caller's transaction for all-or-nothing deletion.
func DeleteAllUserData(ctx context.Context, suid string) error {
	for _, collection := range getUserCollection() {
		if err := ucommon.Delete(
			ctx,
			collection.Name,
			bson.M{collection.SUIDField: suid},
		); err != nil {
			return err
		}
	}
	return nil
}

// All user related collections
func getUserCollection() []collection {
	var collections = []collection{
		{Name: uname.User, SUIDField: suidField},
		{Name: uname.UserAuth, SUIDField: serverUserIDField},
		{Name: uname.UserItem, SUIDField: suidField},
		{Name: uname.UserCard, SUIDField: suidField},
		{Name: uname.UserCharacter, SUIDField: suidField},
		{Name: uname.UserCostume, SUIDField: suidField},
		{Name: uname.UserAccessory, SUIDField: suidField},
		{Name: uname.UserPhoto, SUIDField: suidField},
		{Name: uname.UserPhotoGift, SUIDField: suidField},
		{Name: uname.UserPhotoImageMetadata, SUIDField: suidField},
		{Name: uname.UserDeck, SUIDField: suidField},
		{Name: uname.UserDeckPosition, SUIDField: suidField},
		{Name: uname.UserPoint, SUIDField: suidField},
		{Name: uname.UserStaff, SUIDField: suidField},
		{Name: uname.UserMessage, SUIDField: suidField},
		{Name: uname.UserProfile, SUIDField: suidField},
		{Name: uname.UserEmblem, SUIDField: suidField},
		{Name: uname.UserBalance, SUIDField: suidField},
		{Name: uname.UserTotalCount, SUIDField: suidField},
		{Name: uname.UserPhotoReport, SUIDField: suidField},
		{Name: uname.UserNotification, SUIDField: suidField},
		{Name: uname.UserHierarchy, SUIDField: suidField},
		{Name: uname.UserTutorial, SUIDField: suidField},
		{Name: uname.UserHomePosition, SUIDField: suidField},
		{Name: uname.UserBuddy, SUIDField: suidField},
		{Name: uname.UserHair, SUIDField: suidField},
		{Name: uname.UserCompany, SUIDField: suidField},
		{Name: uname.UserCompanyRewardState, SUIDField: suidField},
		{Name: uname.UserShowcaseLike, SUIDField: suidField},
		{Name: uname.UserPhotoPose, SUIDField: suidField},
		{Name: uname.UserPublic, SUIDField: serverUserIDField},
		{Name: uname.UserStory, SUIDField: suidField},
		{Name: uname.UserTelephone, SUIDField: suidField},
		{Name: uname.UserMission, SUIDField: suidField},
		{Name: uname.UserCharacterMusic, SUIDField: suidField},
		{Name: uname.UserArea, SUIDField: suidField},
		{Name: uname.UserQuest, SUIDField: suidField},
		{Name: uname.UserQuestLiveHistory, SUIDField: suidField},
		{Name: uname.UserQuestLiveReplay, SUIDField: suidField},
		{Name: uname.UserQuestLiveChunk, SUIDField: suidField},
		{Name: uname.UserHomeTalk, SUIDField: suidField},
		{Name: uname.UserCardSupport, SUIDField: suidField},
		{Name: uname.UserMessageSchedule, SUIDField: suidField},
		{Name: uname.UserMusic, SUIDField: suidField},
		{Name: uname.UserDecoration, SUIDField: suidField},
		{Name: uname.UserPhotoRecipe, SUIDField: suidField},
		{Name: uname.UserLove, SUIDField: suidField},
		{Name: uname.UserLoveState, SUIDField: suidField},
		{Name: uname.UserBirthdayState, SUIDField: suidField},
		{Name: uname.UserDreamState, SUIDField: suidField},
		{Name: uname.UserDreamArea, SUIDField: suidField},
		{Name: uname.UserExerciseDeck, SUIDField: suidField},
		{Name: uname.UserSalary, SUIDField: suidField},
		{Name: uname.UserLeagueDeckPosition, SUIDField: suidField},
		{Name: uname.UserDuty, SUIDField: suidField},
		{Name: uname.UserDutyPoint, SUIDField: suidField},
		{Name: uname.UserShowcase, SUIDField: suidField},
		{Name: uname.UserShowcaseLayout, SUIDField: suidField},
		{Name: uname.UserShowcaseMusic, SUIDField: suidField},
		{Name: uname.UserShowcaseToy, SUIDField: suidField},
		{Name: uname.UserShowcaseMyset, SUIDField: suidField},
		{Name: uname.UserShowcaseMysetLayout, SUIDField: suidField},
		{Name: uname.UserTourArea, SUIDField: suidField},
		{Name: uname.UserInvite, SUIDField: suidField},
		{Name: uname.UserActivityFanEventProgress, SUIDField: suidField},
		{Name: uname.UserStaffTrainingReset, SUIDField: suidField},
		{Name: uname.UserShopId, SUIDField: suidField},
		{Name: uname.UserGachaInfo, SUIDField: suidField},
		{Name: uname.UserFanEventInfo, SUIDField: suidField},
		{Name: uname.UserPromotionInfo, SUIDField: suidField},
		{Name: uname.UserRefreshInfo, SUIDField: suidField},
		{Name: uname.UserLessonProgress, SUIDField: suidField},
		{Name: uname.UserLessonInfo, SUIDField: suidField},
		{Name: uname.UserFanEventRun, SUIDField: suidField},
		{Name: uname.UserPromotionRun, SUIDField: suidField},
		{Name: uname.UserRefreshRun, SUIDField: suidField},
		{Name: uname.UserActivityLessonDailyState, SUIDField: suidField},
		{Name: uname.UserFanEventRanking, SUIDField: suidField},
		{Name: uname.UserExchangeBooth, SUIDField: suidField},
		{Name: uname.UserExchangePurchase, SUIDField: suidField},
		{Name: uname.UserShopPurchase, SUIDField: suidField},
		{Name: uname.UserNotiStoneShopItem, SUIDField: suidField},
		{Name: uname.UserNotiNormalShopItem, SUIDField: suidField},
		{Name: uname.UserEventInfo, SUIDField: suidField},
		{Name: uname.UserGodState, SUIDField: suidField},
		{Name: uname.UserHierarchyNotiInfo, SUIDField: suidField},
		{Name: uname.UserEventStoryInfo, SUIDField: suidField},
		{Name: uname.UserPhotoActivity, SUIDField: suidField},
		{Name: uname.UserPhotoMusic, SUIDField: suidField},
		{Name: uname.UserPhotoStage, SUIDField: suidField},
		{Name: uname.UserExtraStoryPartInfo, SUIDField: suidField},
		{Name: uname.UserNotiShowcaseShopItem, SUIDField: suidField},
		{Name: uname.UserShowcaseToyShopId, SUIDField: suidField},
		{Name: uname.UserUnlockedShowcaseNumber, SUIDField: suidField},
		{Name: uname.UserPhotoExpression, SUIDField: suidField},
		{Name: uname.UserCurrentQuestArea, SUIDField: suidField},
		{Name: uname.UserGift, SUIDField: suidField},
		{Name: uname.UserGiftHistory, SUIDField: suidField},
		{Name: uname.UserLoginBonus, SUIDField: suidField},
		{Name: uname.UserDiaryState, SUIDField: suidField},
		{Name: uname.UserMessageProgress, SUIDField: suidField},
		{Name: uname.UserMarketState, SUIDField: suidField},
		{Name: uname.UserExcursionState, SUIDField: suidField},
		{Name: uname.UserExcursionGazeReaction, SUIDField: suidField},
		{Name: uname.UserGachaHistory, SUIDField: suidField},
		{Name: uname.UserAppRequest, SUIDField: suidField},
	}
	return collections
}
