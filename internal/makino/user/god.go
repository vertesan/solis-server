package user

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	messagepath "github.com/vertesan/solis-server/internal/makino/message"
	"github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/makino/user/character"
	"github.com/vertesan/solis-server/internal/makino/user/costume"
	"github.com/vertesan/solis-server/internal/makino/user/tutorial"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	GodMinimumResourceAmount  int64 = 1_000_000
	GodMinimumBalanceAmount   int32 = 999_999
	GodMinimumPointAmount     int64 = 100_000_000_000
	GodMinimumLightFanAmount  int32 = 500_000
	GodMinimumMiddleFanAmount int32 = 300_000
	GodMinimumHeavyFanAmount  int32 = 200_000
	GodMinimumReliability     int32 = 1_000
)

type GodResult struct {
	ServerUserId string
	Collections  map[string]int
}

type GodFeature struct {
	Message string `json:"message,omitempty"`
	Fan     string `json:"fan,omitempty"`
	Bond    string `json:"bond,omitempty"`
}

type godMasterData struct {
	managerLevel     *pmaster.ManagerLevel
	characterLevel   *pmaster.CharacterActivityLevel
	cardSupport      *pmaster.CardSupportRelease
	setting          *pmaster.Setting
	activityCatalog  *godActivityCatalog
	accessories      []*pmaster.Accessory
	activityFans     []*pmaster.ActivityFanEvent
	activityPromos   []*pmaster.ActivityPromotion
	activityRefresh  []*pmaster.ActivityRefresh
	areas            []*pmaster.Area
	cards            []*pmaster.Card
	characters       []*pmaster.Character
	companyGrades    []*pmaster.CompanyGrade
	companyLevels    []*pmaster.CompanyEnjoyCharacterLevel
	companySettings  []*pmaster.CompanyEnjoyCharacterLevelSetting
	companyTrust     []*pmaster.CompanyTrustLevel
	conditions       []*pmaster.Condition
	costumes         []*pmaster.Costume
	decorations      []*pmaster.Decoration
	emblems          []*pmaster.Emblem
	eventStories     []*pmaster.EventStory
	extraStories     []*pmaster.ExtraStory
	extraStoryParts  []*pmaster.ExtraStoryPart
	excursionGazes   []*pmaster.ExcursionGazeReaction
	hairs            []*pmaster.Hair
	homeTalks        []*pmaster.HomeTalk
	items            []*pmaster.Item
	loveHomeActions  []*pmaster.LoveHomeAction
	loveLevels       []*pmaster.LoveLevel
	messages         []*pmaster.Message
	missions         []*pmaster.Mission
	musics           []*pmaster.Music
	photoActivities  []*pmaster.PhotoActivity
	photoExpressions []*pmaster.PhotoExpression
	photoMusics      []*pmaster.PhotoQuestMusic
	photoPoses       []*pmaster.PhotoPose
	photoRecipes     []*pmaster.PhotoRecipe
	photoStages      []*pmaster.PhotoQuestStage
	quests           []*pmaster.Quest
	showcaseMusics   []*pmaster.ShowcaseMusic
	showcaseToys     []*pmaster.ShowcaseToy
	staffLevels      []*pmaster.StaffLevel
	staffRanks       []*pmaster.StaffLevelLimitBreakRank
	stories          []*pmaster.Story
	storyParts       []*pmaster.StoryPart
	telephones       []*pmaster.Telephone
	tutorials        []*ptransaction.UserTutorial
	userCards        []*ptransaction.UserCard
	userCharacters   []*ptransaction.UserCharacter
	userCostumes     []*ptransaction.UserCostume
}

func GodUser(ctx context.Context, publicUserId, serverUserId string, feature GodFeature) (*GodResult, error) {
	if (publicUserId == "") == (serverUserId == "") {
		return nil, fmt.Errorf("exactly one of publicUserId or serverUserId is required")
	}
	var err error
	feature.Message, err = normalizeGodFeature("message", feature.Message)
	if err != nil {
		return nil, err
	}
	feature.Fan, err = normalizeGodFeature("fan", feature.Fan)
	if err != nil {
		return nil, err
	}
	feature.Bond, err = normalizeGodFeature("bond", feature.Bond)
	if err != nil {
		return nil, err
	}

	auth, err := findGodUserAuth(ctx, publicUserId, serverUserId)
	if err != nil {
		return nil, err
	}
	masters, err := loadGodMasterData(ctx)
	if err != nil {
		return nil, err
	}

	result := &GodResult{ServerUserId: auth.ServerUserId, Collections: make(map[string]int)}
	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := opuser.FindUser(ctx, auth.ServerUserId); err != nil {
			return nil, err
		}
		if err := applyGodMasterData(ctx, auth.ServerUserId, masters, feature, result.Collections); err != nil {
			return nil, err
		}
		return nil, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to unlock user: %w", err)
	}
	return result, nil
}

func normalizeGodFeature(name, value string) (string, error) {
	if value == "" {
		return umodel.GodMessageFeatureAll, nil
	}
	switch value {
	case umodel.GodMessageFeatureNone, umodel.GodMessageFeatureAll, umodel.GodMessageFeatureEnjoy:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported %s feature %q", name, value)
	}
}

func findGodUserAuth(ctx context.Context, publicUserId, serverUserId string) (*umodel.UserAuth, error) {
	if publicUserId != "" {
		return opuser.GetUserAuthByPublicId(ctx, publicUserId)
	}
	return opuser.GetUserAuthByServerId(ctx, serverUserId)
}

func loadGodMasterData(ctx context.Context) (*godMasterData, error) {
	activityCatalog, err := loadGodActivityCatalog()
	if err != nil {
		return nil, err
	}
	data := &godMasterData{activityCatalog: activityCatalog}
	settingId, err := config.GetGameSettingId()
	if err != nil {
		return nil, err
	}
	group, groupCtx := errgroup.WithContext(ctx)
	run := func(name string, load func() error) {
		group.Go(func() error {
			if err := load(); err != nil {
				return fmt.Errorf("failed to load %s master data: %w", name, err)
			}
			return nil
		})
	}

	run("ManagerLevel", func() (err error) {
		data.managerLevel, err = opmaster.FindMaxManagerLevel(groupCtx)
		return err
	})
	run("CardSupportRelease", func() (err error) {
		data.cardSupport, err = opmaster.FindMaxCardSupportRelease(groupCtx)
		return err
	})
	run("CharacterActivityLevel", func() (err error) {
		data.characterLevel, err = opmaster.FindMaxCharacterActivityLevel(groupCtx)
		return err
	})
	run("Setting", func() (err error) {
		data.setting, err = opmaster.FindSetting(groupCtx, settingId)
		return err
	})
	run("Accessory", func() (err error) {
		data.accessories, err = opmaster.FindAllAccessories(groupCtx)
		return err
	})
	run("ActivityFanEvent", func() (err error) {
		data.activityFans, err = opmaster.FindAllActivityFanEvents(groupCtx)
		return err
	})
	run("ActivityPromotion", func() (err error) {
		data.activityPromos, err = opmaster.FindAllActivityPromotions(groupCtx)
		return err
	})
	run("ActivityRefresh", func() (err error) {
		data.activityRefresh, err = opmaster.FindAllActivityRefreshes(groupCtx)
		return err
	})
	run("Area", func() (err error) {
		data.areas, err = opmaster.FindAllAreas(groupCtx)
		return err
	})
	run("Card", func() (err error) {
		data.cards, err = opmaster.FindAllCards(groupCtx)
		return err
	})
	run("Character", func() (err error) {
		data.characters, err = opmaster.FindAllCharacters(groupCtx)
		return err
	})
	run("CompanyGrade", func() (err error) {
		data.companyGrades, err = opmaster.FindAllCompanyGrades(groupCtx)
		return err
	})
	run("CompanyEnjoyCharacterLevel", func() (err error) {
		data.companyLevels, err = opmaster.FindAllCompanyEnjoyCharacterLevels(groupCtx)
		return err
	})
	run("CompanyEnjoyCharacterLevelSetting", func() (err error) {
		data.companySettings, err = opmaster.FindAllCompanyEnjoyCharacterLevelSettings(groupCtx)
		return err
	})
	run("CompanyTrustLevel", func() (err error) {
		data.companyTrust, err = opmaster.FindAllCompanyTrustLevels(groupCtx)
		return err
	})
	run("Condition", func() (err error) {
		data.conditions, err = opmaster.FindAllConditions(groupCtx)
		return err
	})
	run("Costume", func() (err error) {
		data.costumes, err = opmaster.FindAllCostumes(groupCtx)
		return err
	})
	run("Decoration", func() (err error) {
		data.decorations, err = opmaster.FindAllDecorations(groupCtx)
		return err
	})
	run("Emblem", func() (err error) {
		data.emblems, err = opmaster.FindAllEmblems(groupCtx)
		return err
	})
	run("EventStory", func() (err error) {
		data.eventStories, err = opmaster.FindAllEventStories(groupCtx)
		return err
	})
	run("ExtraStory", func() (err error) {
		data.extraStories, err = opmaster.FindAllExtraStories(groupCtx)
		return err
	})
	run("ExtraStoryPart", func() (err error) {
		data.extraStoryParts, err = opmaster.FindAllExtraStoryParts(groupCtx)
		return err
	})
	run("ExcursionGazeReaction", func() (err error) {
		data.excursionGazes, err = opmaster.FindAllExcursionGazeReactions(groupCtx)
		return err
	})
	run("Hair", func() (err error) {
		data.hairs, err = opmaster.FindAllHairs(groupCtx)
		return err
	})
	run("HomeTalk", func() (err error) {
		data.homeTalks, err = opmaster.FindAllHomeTalks(groupCtx)
		return err
	})
	run("Item", func() (err error) {
		data.items, err = opmaster.FindAllItems(groupCtx)
		return err
	})
	run("Music", func() (err error) {
		data.musics, err = opmaster.FindAllMusics(groupCtx)
		return err
	})
	run("Message", func() (err error) {
		data.messages, err = opmaster.FindAllMessages(groupCtx)
		return err
	})
	run("Mission", func() (err error) {
		data.missions, err = opmaster.FindAllMissions(groupCtx)
		return err
	})
	run("LoveLevel", func() (err error) {
		data.loveLevels, err = opmaster.FindMaxLoveLevels(groupCtx)
		return err
	})
	run("LoveHomeAction", func() (err error) {
		data.loveHomeActions, err = opmaster.FindAllLoveHomeActions(groupCtx)
		return err
	})
	run("PhotoActivity", func() (err error) {
		data.photoActivities, err = opmaster.FindAllPhotoActivities(groupCtx)
		return err
	})
	run("PhotoExpression", func() (err error) {
		data.photoExpressions, err = opmaster.FindAllPhotoExpressions(groupCtx)
		return err
	})
	run("PhotoQuestMusic", func() (err error) {
		data.photoMusics, err = opmaster.FindAllPhotoQuestMusics(groupCtx)
		return err
	})
	run("PhotoPose", func() (err error) {
		data.photoPoses, err = opmaster.FindAllPhotoPoses(groupCtx)
		return err
	})
	run("PhotoRecipe", func() (err error) {
		data.photoRecipes, err = opmaster.FindAllPhotoRecipes(groupCtx)
		return err
	})
	run("PhotoQuestStage", func() (err error) {
		data.photoStages, err = opmaster.FindAllPhotoQuestStages(groupCtx)
		return err
	})
	run("Quest", func() (err error) {
		data.quests, err = opmaster.FindAllQuests(groupCtx)
		return err
	})
	run("ShowcaseMusic", func() (err error) {
		data.showcaseMusics, err = opmaster.FindAllShowcaseMusics(groupCtx)
		return err
	})
	run("ShowcaseToy", func() (err error) {
		data.showcaseToys, err = opmaster.FindAllShowcaseToys(groupCtx)
		return err
	})
	run("StaffLevel", func() (err error) {
		data.staffLevels, err = opmaster.FindMaxStaffLevels(groupCtx)
		return err
	})
	run("StaffLevelLimitBreakRank", func() (err error) {
		data.staffRanks, err = opmaster.FindMaxStaffLimitBreakRanks(groupCtx)
		return err
	})
	run("Story", func() (err error) {
		data.stories, err = opmaster.FindAllStories(groupCtx)
		return err
	})
	run("StoryPart", func() (err error) {
		data.storyParts, err = opmaster.FindAllStoryParts(groupCtx)
		return err
	})
	run("Telephone", func() (err error) {
		data.telephones, err = opmaster.FindAllTelephones(groupCtx)
		return err
	})
	run("Tutorial", func() (err error) {
		data.tutorials, err = tutorial.SpawnUserInitialTutorial(groupCtx)
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}
	if err := validateGodActivityCatalog(
		ctx,
		data.activityCatalog,
		data.areas,
		data.activityFans,
		data.activityPromos,
		data.activityRefresh,
	); err != nil {
		return nil, fmt.Errorf("failed to validate god activity catalog: %w", err)
	}

	data.userCards, err = spawnGodCards(ctx, data.cards)
	if err != nil {
		return nil, err
	}
	for _, masterCharacter := range data.characters {
		if !character.IsPlayableCharacter(masterCharacter) {
			continue
		}
		userCharacter, err := character.SpawnUserCharacter(ctx, masterCharacter)
		if err != nil {
			return nil, fmt.Errorf("failed to spawn character %q: %w", masterCharacter.Id, err)
		}
		data.userCharacters = append(data.userCharacters, userCharacter)
	}
	for _, masterCostume := range data.costumes {
		data.userCostumes = append(data.userCostumes, costume.SpawnUserCostume(masterCostume))
	}
	return data, nil
}

func spawnGodCards(ctx context.Context, cards []*pmaster.Card) ([]*ptransaction.UserCard, error) {
	result := make([]*ptransaction.UserCard, len(cards))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(16)
	for index, masterCard := range cards {
		group.Go(func() error {
			userCard, err := card.SpawnUserCard(groupCtx, masterCard)
			if err != nil {
				return fmt.Errorf("failed to spawn card %q: %w", masterCard.Id, err)
			}
			result[index] = userCard
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return result, nil
}

func applyGodMasterData(
	ctx context.Context,
	suid string,
	data *godMasterData,
	feature GodFeature,
	counts map[string]int,
) error {
	now := time.Now().UnixMilli()
	userData, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return err
	}
	userData.ManagerLevel = data.managerLevel.Level
	userData.ManagerExp = data.managerLevel.RequiredExp
	userData.CardSupportLevel = data.setting.CardSupportLevelLimit
	userData.CardSupportReleaseCount = data.cardSupport.Count
	userData.CardSupportMaxNumber = data.cardSupport.Count + 3
	userData.PhotoLimitExtendedCount = max(
		userData.GetPhotoLimitExtendedCount(),
		data.setting.GetPhotoLimitExtendLimit(),
	)
	if err := opuser.UpsertUser(ctx, suid, userData); err != nil {
		return err
	}
	counts[uname.User] = 1

	if err := applyGodOwnership(ctx, suid, data, now, feature, counts); err != nil {
		return err
	}
	if err := applyGodProgress(ctx, suid, data, now, counts); err != nil {
		return err
	}
	if err := applyGodCompany(ctx, suid, data, now, feature, counts); err != nil {
		return err
	}
	if err := applyGodFeatureGates(ctx, suid, data, feature, counts); err != nil {
		return err
	}
	if err := applyGodResources(ctx, suid, data, counts); err != nil {
		return err
	}
	if err := opuser.UpsertUserDiaryState(ctx, &umodel.UserDiaryState{SUID: suid, AllRead: true}); err != nil {
		return err
	}
	counts[uname.UserDiaryState] = 1
	return nil
}

func applyGodFeatureGates(
	ctx context.Context,
	suid string,
	data *godMasterData,
	feature GodFeature,
	counts map[string]int,
) error {
	fanInfo, err := opuser.FindUserFanEventInfo(ctx, suid)
	if err != nil {
		return err
	}
	fansById := make(map[string]*papi.FanEvent, len(fanInfo.FanEvents))
	for _, value := range fanInfo.FanEvents {
		fansById[value.Id] = value
	}
	fanEvents := make([]*papi.FanEvent, 0, len(data.activityFans))
	for _, master := range data.activityFans {
		profile := data.activityCatalog.fanEvent(master.GetId())
		highestLevel := maxFanEventLevel(master)
		if profile == nil || highestLevel == nil {
			return fmt.Errorf("validated fan event %q profile is unavailable", master.GetId())
		}
		var exp int32
		if existing := fansById[master.GetId()]; existing != nil {
			exp = existing.GetExp()
		}
		fanEvents = append(fanEvents, &papi.FanEvent{
			Id:                         master.GetId(),
			Name:                       master.GetName(),
			SubName:                    highestLevel.GetName(),
			Type:                       master.GetType(),
			UnlockConditionId:          master.GetUnlockConditionId(),
			IsUnlocked:                 true,
			ActivityPerformanceId:      profile.ActivityPerformanceID,
			Level:                      profile.Level,
			Exp:                        exp,
			NextLevelRequiredExp:       0,
			RequiredStamina:            profile.RequiredStamina,
			NextLevelUnlockConditionId: "",
			IsNextLevelUnlocked:        false,
			StepConsumptionStamina:     profile.StepConsumptionStamina,
			FixedRewardId:              profile.FixedRewardID,
			AdditionalRewardId:         "",
			RankSRewardId:              "",
			RankARewardId:              "",
			RankBRewardId:              "",
			RankCRewardId:              "",
			RankDRewardId:              "",
			ViewConditionId:            master.GetViewConditionId(),
			CampaignDropRewardIds:      []string{},
		})
	}
	fanInfo.FanEvents = fanEvents
	fanInfo.CampaignEffects = []*papi.ActivityCampaignEffect{}
	if err := opuser.UpsertUserFanEventInfo(ctx, suid, fanInfo); err != nil {
		return err
	}
	counts[uname.UserFanEventInfo] = len(fanInfo.FanEvents)

	promotionInfo, err := opuser.FindUserPromotionInfo(ctx, suid)
	if err != nil {
		return err
	}
	promotionsById := make(map[string]*papi.Promotion, len(promotionInfo.Promotions))
	for _, value := range promotionInfo.Promotions {
		promotionsById[value.Id] = value
	}
	promotions := make([]*papi.Promotion, 0, len(data.activityPromos))
	for _, master := range data.activityPromos {
		profile := data.activityCatalog.promotion(master.GetId())
		highestLevel := maxPromotionLevel(master)
		if profile == nil || highestLevel == nil {
			return fmt.Errorf("validated promotion %q profile is unavailable", master.GetId())
		}
		var exp int32
		if existing := promotionsById[master.GetId()]; existing != nil {
			exp = existing.GetExp()
		}
		promotions = append(promotions, &papi.Promotion{
			Id:                             master.GetId(),
			Name:                           master.GetName(),
			SubName:                        highestLevel.GetName(),
			UnlockConditionId:              master.GetUnlockConditionId(),
			IsUnlocked:                     true,
			ActivityPerformanceId:          profile.ActivityPerformanceID,
			Level:                          profile.Level,
			Exp:                            exp,
			NextLevelRequiredExp:           0,
			RequiredStamina:                profile.RequiredStamina,
			NextLevelUnlockConditionId:     "",
			IsNextLevelUnlocked:            false,
			StepConsumptionStamina:         profile.StepConsumptionStamina,
			StepRewardGold:                 profile.StepRewardGold,
			StepRewardCardEnhanceItem:      profile.StepRewardCardEnhanceItem,
			StepAdditionalRewardId:         "",
			MultiStepRewardId:              profile.MultiStepRewardID,
			MaxFanAmount:                   profile.MaxFanAmount,
			ViewConditionId:                master.GetViewConditionId(),
			SubGenre:                       profile.SubGenre,
			MultiStepCampaignDropRewardIds: []string{},
		})
	}
	promotionInfo.Promotions = promotions
	promotionInfo.CampaignEffects = []*papi.ActivityCampaignEffect{}
	if err := opuser.UpsertUserPromotionInfo(ctx, suid, promotionInfo); err != nil {
		return err
	}
	counts[uname.UserPromotionInfo] = len(promotionInfo.Promotions)

	refreshInfo, err := opuser.FindUserRefreshInfo(ctx, suid)
	if err != nil {
		return err
	}
	refreshesById := make(map[string]*papi.Refresh, len(refreshInfo.Refreshes))
	for _, value := range refreshInfo.Refreshes {
		refreshesById[value.Id] = value
	}
	refreshes := make([]*papi.Refresh, 0, len(data.activityRefresh))
	for _, master := range data.activityRefresh {
		profile := data.activityCatalog.refresh(master.GetId())
		if profile == nil {
			return fmt.Errorf("validated refresh %q profile is unavailable", master.GetId())
		}
		levels := make([]*papi.RefreshLevel, 0, len(master.GetLevels()))
		var highestLevel int32
		for _, masterLevel := range master.GetLevels() {
			if masterLevel.GetLevel() > highestLevel {
				highestLevel = masterLevel.GetLevel()
			}
			levelProfile := profile.level(masterLevel.GetLevel())
			if levelProfile == nil {
				return fmt.Errorf(
					"validated refresh %q level %d profile is unavailable",
					master.GetId(),
					masterLevel.GetLevel(),
				)
			}
			levels = append(levels, &papi.RefreshLevel{
				Level:                 masterLevel.GetLevel(),
				Name:                  masterLevel.GetName(),
				RequiredGold:          levelProfile.RequiredGold,
				UnlockConditionId:     masterLevel.GetUnlockConditionId(),
				IsUnlocked:            true,
				ActivityPerformanceId: levelProfile.ActivityPerformanceID,
				RecoveryAmount:        levelProfile.RecoveryAmount,
				ViewConditionId:       masterLevel.GetViewConditionId(),
				SubGenre:              levelProfile.SubGenre,
			})
		}
		var exp int32
		if existing := refreshesById[master.GetId()]; existing != nil {
			exp = existing.GetExp()
		}
		refreshes = append(refreshes, &papi.Refresh{
			Id:                   master.GetId(),
			Name:                 master.GetName(),
			RequiredMinutes:      profile.RequiredMinutes,
			UnlockConditionId:    master.GetUnlockConditionId(),
			IsUnlocked:           true,
			Level:                highestLevel,
			Exp:                  exp,
			NextLevelRequiredExp: 0,
			Levels:               levels,
			ViewConditionId:      master.GetViewConditionId(),
		})
	}
	refreshInfo.Refreshes = refreshes
	refreshInfo.CampaignEffects = []*papi.ActivityCampaignEffect{}
	if err := opuser.UpsertUserRefreshInfo(ctx, suid, refreshInfo); err != nil {
		return err
	}
	counts[uname.UserRefreshInfo] = len(refreshInfo.Refreshes)
	if err := opuser.UpsertUserLessonInfo(ctx, suid, &papi.LessonInfo{
		CampaignEffects:       []*papi.ActivityCampaignEffect{},
		CampaignDropRewardIds: []string{},
	}); err != nil {
		return err
	}
	counts[uname.UserLessonInfo] = 1

	current, err := opuser.FindUserCurrentQuestArea(ctx, suid)
	if err != nil {
		return err
	}
	mainArea, towerArea := furthestGodAreas(data.areas)
	if mainArea != nil {
		current.CurrentMainAreaId = data.activityCatalog.Lesson.CurrentAreaID
		lesson, err := opuser.FindUserLessonProgress(ctx, suid)
		if err != nil {
			return err
		}
		lesson.CurrentAreaId = data.activityCatalog.Lesson.CurrentAreaID
		lesson.StepRewardGold = data.activityCatalog.Lesson.StepRewardGold
		lesson.StepRewardManagerExp = data.activityCatalog.Lesson.StepRewardManagerExp
		lesson.StepRewardCardEnhanceItem = data.activityCatalog.Lesson.StepRewardCardEnhanceItem
		if err := opuser.UpsertUserLessonProgress(ctx, suid, lesson); err != nil {
			return err
		}
		counts[uname.UserLessonProgress] = 1
	}
	if towerArea != nil && len(towerArea.QuestIds) > 0 {
		current.CurrentTowerQuestId = towerArea.QuestIds[len(towerArea.QuestIds)-1]
	}
	if err := opuser.UpsertUserCurrentQuestArea(ctx, current); err != nil {
		return err
	}
	counts[uname.UserCurrentQuestArea] = 1

	if err := opuser.UpsertUserGodState(ctx, &umodel.UserGodState{
		SUID:                suid,
		UnlockCurrentEvents: true,
		MessageFeature:      feature.Message,
	}); err != nil {
		return err
	}
	// Older god-mode builds persisted a complete HomeEventInfo snapshot. It is
	// no longer authoritative because it would pin the user's event rotation.
	if err := opuser.DeleteUserEventInfo(ctx, suid); err != nil {
		return err
	}
	counts[uname.UserGodState] = 1
	return nil
}

func furthestGodAreas(areas []*pmaster.Area) (mainArea, towerArea *pmaster.Area) {
	for _, area := range areas {
		switch area.Type {
		case penum.AreaType_AreaType_Main:
			if mainArea == nil || area.Order > mainArea.Order || area.Order == mainArea.Order && area.Id > mainArea.Id {
				mainArea = area
			}
		case penum.AreaType_AreaType_Tower:
			if towerArea == nil || area.Order > towerArea.Order || area.Order == towerArea.Order && area.Id > towerArea.Id {
				towerArea = area
			}
		}
	}
	return mainArea, towerArea
}

func applyGodOwnership(
	ctx context.Context,
	suid string,
	data *godMasterData,
	now int64,
	feature GodFeature,
	counts map[string]int,
) error {
	cards := make([]opuser.GodUpsert[*ptransaction.UserCard], 0, len(data.userCards))
	for _, value := range data.userCards {
		cards = append(cards, godValue("cardId", value.CardId, value))
	}
	if err := godUpsert(ctx, uname.UserCard, suid, cards, false, counts); err != nil {
		return err
	}

	characters := make([]opuser.GodUpsert[*ptransaction.UserCharacter], 0, len(data.userCharacters))
	playableCharacterIDs := make(map[string]struct{}, len(data.userCharacters))
	for _, value := range data.userCharacters {
		characters = append(characters, godValue("characterId", value.CharacterId, value))
		playableCharacterIDs[value.GetCharacterId()] = struct{}{}
	}
	if err := godUpsert(ctx, uname.UserCharacter, suid, characters, false, counts); err != nil {
		return err
	}
	ownedCharacters, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		return err
	}
	for _, value := range ownedCharacters {
		if value.ActivityLevel < data.characterLevel.Level {
			value.ActivityLevel = data.characterLevel.Level
		}
		if value.ActivityExp < int64(data.characterLevel.RequiredExp) {
			value.ActivityExp = int64(data.characterLevel.RequiredExp)
		}
		if !value.InActivity && value.ActivityStamina < data.characterLevel.MaxStamina {
			value.ActivityStamina = data.characterLevel.MaxStamina
			value.StaminaUpdatedTime = now
		}
		_, playable := playableCharacterIDs[value.GetCharacterId()]
		if playable && feature.Fan == umodel.GodMessageFeatureAll {
			value.LightFanAmount = max(value.GetLightFanAmount(), GodMinimumLightFanAmount)
			value.MiddleFanAmount = max(value.GetMiddleFanAmount(), GodMinimumMiddleFanAmount)
			value.HeavyFanAmount = max(value.GetHeavyFanAmount(), GodMinimumHeavyFanAmount)
			audience, err := godAudienceCandidateAmount(data.setting, value)
			if err != nil {
				return err
			}
			value.AudienceCandidateAmount = audience
		}
		if playable && feature.Bond == umodel.GodMessageFeatureAll {
			value.Reliability = max(value.GetReliability(), GodMinimumReliability)
		}
		if err := opuser.UpsertUserCharacter(ctx, suid, value); err != nil {
			return err
		}
	}
	counts[uname.UserCharacter] = len(ownedCharacters)

	costumes := make([]opuser.GodUpsert[*ptransaction.UserCostume], 0, len(data.userCostumes))
	for _, value := range data.userCostumes {
		costumes = append(costumes, godValue("costumeId", value.CostumeId, value))
	}
	if err := godUpsert(ctx, uname.UserCostume, suid, costumes, false, counts); err != nil {
		return err
	}

	hairs := make([]opuser.GodUpsert[*ptransaction.UserHair], 0, len(data.hairs))
	for _, master := range data.hairs {
		hairs = append(hairs, godValue("hairId", master.Id, &ptransaction.UserHair{HairId: master.Id}))
	}
	if err := godUpsert(ctx, uname.UserHair, suid, hairs, false, counts); err != nil {
		return err
	}

	decorations := make([]opuser.GodUpsert[*ptransaction.UserDecoration], 0, len(data.decorations))
	for _, master := range data.decorations {
		decorations = append(decorations, godValue("decorationId", master.Id, &ptransaction.UserDecoration{DecorationId: master.Id}))
	}
	if err := godUpsert(ctx, uname.UserDecoration, suid, decorations, false, counts); err != nil {
		return err
	}

	emblems := make([]opuser.GodUpsert[*ptransaction.UserEmblem], 0, len(data.emblems))
	for _, master := range data.emblems {
		emblems = append(emblems, godValue("emblemId", master.Id, &ptransaction.UserEmblem{EmblemId: master.Id}))
	}
	if err := godUpsert(ctx, uname.UserEmblem, suid, emblems, false, counts); err != nil {
		return err
	}

	return applyGodMediaOwnership(ctx, suid, data, now, feature, counts)
}

func godAudienceCandidateAmount(setting *pmaster.Setting, value *ptransaction.UserCharacter) (int32, error) {
	amount := int64(value.GetLightFanAmount())*int64(setting.GetLiveLightFanAmountWeightPermil())/1000 +
		int64(value.GetMiddleFanAmount())*int64(setting.GetLiveMiddleFanAmountWeightPermil())/1000 +
		int64(value.GetHeavyFanAmount())*int64(setting.GetLiveHeavyFanAmountWeightPermil())/1000
	if amount < 0 || amount > math.MaxInt32 {
		return 0, fmt.Errorf("god audience candidate amount is invalid for character %q", value.GetCharacterId())
	}
	return int32(amount), nil
}

func applyGodMediaOwnership(
	ctx context.Context,
	suid string,
	data *godMasterData,
	now int64,
	feature GodFeature,
	counts map[string]int,
) error {
	musics := make([]opuser.GodUpsert[*ptransaction.UserMusic], 0, len(data.musics))
	characterMusics := make([]opuser.GodUpsert[*ptransaction.UserCharacterMusic], 0, len(data.musics)*len(data.userCharacters))
	for _, music := range data.musics {
		musics = append(musics, godValue("musicId", music.Id, &ptransaction.UserMusic{MusicId: music.Id}))
		for _, character := range data.userCharacters {
			characterMusic := &ptransaction.UserCharacterMusic{CharacterId: character.CharacterId, MusicId: music.Id}
			characterMusics = append(characterMusics, opuser.GodUpsert[*ptransaction.UserCharacterMusic]{
				Key: bson.M{"characterId": character.CharacterId, "musicId": music.Id}, Data: characterMusic,
			})
		}
	}
	if err := godUpsert(ctx, uname.UserMusic, suid, musics, false, counts); err != nil {
		return err
	}
	loveStoryEndIds := make(map[string][]string, len(data.loveLevels))
	seenLoveStoryEndIds := make(map[string]map[string]struct{}, len(data.loveLevels))
	for _, action := range data.loveHomeActions {
		if _, exists := seenLoveStoryEndIds[action.LoveId]; !exists {
			seenLoveStoryEndIds[action.LoveId] = make(map[string]struct{})
		}
		for _, storyEndId := range action.UnlockLoveStoryEndIds {
			if _, exists := seenLoveStoryEndIds[action.LoveId][storyEndId]; exists {
				continue
			}
			seenLoveStoryEndIds[action.LoveId][storyEndId] = struct{}{}
			loveStoryEndIds[action.LoveId] = append(loveStoryEndIds[action.LoveId], storyEndId)
		}
	}
	existingLoves, err := opuser.FindAllUserLoves(ctx, suid)
	if err != nil {
		return err
	}
	existingLovesById := make(map[string]*ptransaction.UserLove, len(existingLoves))
	for _, value := range existingLoves {
		existingLovesById[value.LoveId] = value
	}
	loves := make([]opuser.GodUpsert[*ptransaction.UserLove], 0, len(data.loveLevels))
	for _, master := range data.loveLevels {
		value := &ptransaction.UserLove{LoveId: master.LoveId}
		if existing := existingLovesById[master.LoveId]; existing != nil {
			value = proto.Clone(existing).(*ptransaction.UserLove)
		}
		if value.HiddenFavorability < master.RequiredExp {
			value.HiddenFavorability = master.RequiredExp
		}
		seenEndIds := make(map[string]struct{}, len(value.UnlockedLoveStoryEndIDs)+len(loveStoryEndIds[master.LoveId]))
		mergedEndIds := make([]string, 0, len(value.UnlockedLoveStoryEndIDs)+len(loveStoryEndIds[master.LoveId]))
		for _, storyEndId := range value.UnlockedLoveStoryEndIDs {
			if _, exists := seenEndIds[storyEndId]; exists {
				continue
			}
			seenEndIds[storyEndId] = struct{}{}
			mergedEndIds = append(mergedEndIds, storyEndId)
		}
		for _, storyEndId := range loveStoryEndIds[master.LoveId] {
			if _, exists := seenEndIds[storyEndId]; exists {
				continue
			}
			seenEndIds[storyEndId] = struct{}{}
			mergedEndIds = append(mergedEndIds, storyEndId)
		}
		value.UnlockedLoveStoryEndIDs = mergedEndIds
		loves = append(loves, godValue("loveId", master.LoveId, value))
	}
	if err := godUpsert(ctx, uname.UserLove, suid, loves, true, counts); err != nil {
		return err
	}
	if err := godUpsert(ctx, uname.UserCharacterMusic, suid, characterMusics, false, counts); err != nil {
		return err
	}

	homeTalks := make([]opuser.GodUpsert[*ptransaction.UserHomeTalk], 0, len(data.homeTalks))
	for _, master := range data.homeTalks {
		homeTalks = append(homeTalks, godValue("homeTalkId", master.HomeTalkId, &ptransaction.UserHomeTalk{HomeTalkId: master.HomeTalkId}))
	}
	if err := godUpsert(ctx, uname.UserHomeTalk, suid, homeTalks, false, counts); err != nil {
		return err
	}

	stories := make([]opuser.GodUpsert[*ptransaction.UserStory], 0, len(data.stories))
	for _, master := range data.stories {
		stories = append(stories, godValue("storyId", master.Id, &ptransaction.UserStory{
			StoryId: master.Id, StatusType: penum.StoryStatusType_StoryStatusType_Unread,
		}))
	}
	if err := godUpsert(ctx, uname.UserStory, suid, stories, false, counts); err != nil {
		return err
	}
	if err := opuser.MarkUserStoriesReadForGod(ctx, suid, mainGodStoryIds(data.storyParts)); err != nil {
		return err
	}
	if err := applyGodNarrative(ctx, suid, data, now, feature.Message, counts); err != nil {
		return err
	}

	showcaseMusics := make([]opuser.GodUpsert[*ptransaction.UserShowcaseMusic], 0, len(data.showcaseMusics))
	for _, master := range data.showcaseMusics {
		showcaseMusics = append(showcaseMusics, godValue("showcaseMusicId", master.MusicId, &ptransaction.UserShowcaseMusic{ShowcaseMusicId: master.MusicId}))
	}
	if err := godUpsert(ctx, uname.UserShowcaseMusic, suid, showcaseMusics, false, counts); err != nil {
		return err
	}

	showcaseToys := make([]opuser.GodUpsert[*ptransaction.UserShowcaseToy], 0, len(data.showcaseToys))
	for _, master := range data.showcaseToys {
		showcaseToys = append(showcaseToys, godValue("showcaseToyId", master.Id, &ptransaction.UserShowcaseToy{ShowcaseToyId: master.Id, Amount: 1}))
	}
	return godUpsert(ctx, uname.UserShowcaseToy, suid, showcaseToys, false, counts)
}

func applyGodNarrative(
	ctx context.Context,
	suid string,
	data *godMasterData,
	now int64,
	messageFeature string,
	counts map[string]int,
) error {
	if messageFeature == umodel.GodMessageFeatureNone {
		return nil
	}

	embeddedTelephoneIDs := make(map[string]struct{})
	for _, master := range data.messages {
		for _, detail := range master.GetDetails() {
			if detail.GetTelephoneId() != "" {
				embeddedTelephoneIDs[detail.GetTelephoneId()] = struct{}{}
			}
		}
	}

	if messageFeature == umodel.GodMessageFeatureEnjoy {
		messages := make([]opuser.GodUpsert[*ptransaction.UserMessage], 0)
		for _, master := range data.messages {
			if master.GetUnlockConditionId() != "" {
				continue
			}
			messages = append(messages, godValue("messageId", master.GetId(), &ptransaction.UserMessage{
				MessageId:         master.GetId(),
				MessageStatusType: penum.MessageStatusType_MessageStatusType_Unlocked,
			}))
		}
		if err := godUpsert(ctx, uname.UserMessage, suid, messages, false, counts); err != nil {
			return err
		}
		telephones := make([]opuser.GodUpsert[*ptransaction.UserTelephone], 0)
		for _, master := range data.telephones {
			if master.GetUnlockConditionId() != "" {
				continue
			}
			if _, embedded := embeddedTelephoneIDs[master.GetId()]; embedded {
				continue
			}
			telephones = append(telephones, godValue("telephoneId", master.GetId(), &ptransaction.UserTelephone{
				TelephoneId:         master.GetId(),
				UnlockedTime:        now,
				TelephoneStatusType: penum.TelephoneStatusType_TelephoneStatusType_Unlocked,
			}))
		}
		return godUpsert(ctx, uname.UserTelephone, suid, telephones, false, counts)
	}

	existingMessages, err := opuser.FindAllUserMessages(ctx, suid)
	if err != nil {
		return err
	}
	existingMessagesByID := make(map[string]*ptransaction.UserMessage, len(existingMessages))
	for _, message := range existingMessages {
		existingMessagesByID[message.GetMessageId()] = message
	}
	existingProgresses, err := opuser.FindAllUserMessageProgresses(ctx, suid)
	if err != nil {
		return err
	}
	existingProgressesByID := make(map[string]*umodel.UserMessageProgress, len(existingProgresses))
	for _, progress := range existingProgresses {
		existingProgressesByID[progress.MessageId] = progress
	}
	messages := make([]opuser.GodUpsert[*ptransaction.UserMessage], 0, len(data.messages))
	progresses := make([]*umodel.UserMessageProgress, 0, len(data.messages))
	for _, master := range data.messages {
		selections, terminal, err := messagepath.FirstPath(master)
		if err != nil {
			return fmt.Errorf("failed to resolve god message %q: %w", master.GetId(), err)
		}
		arrivedTime, finishedTime := now, now
		existing := existingMessagesByID[master.GetId()]
		if existing != nil && existing.GetArrivedTime() > 0 && existing.GetArrivedTime() <= now {
			arrivedTime = existing.GetArrivedTime()
		}
		if existing != nil && existing.GetMessageStatusType() == penum.MessageStatusType_MessageStatusType_Finished {
			existingTerminal, historyErr := messagepath.ExistingHistoryPath(master, existing.GetSelectMessageDetailIds())
			if historyErr == nil {
				selections = append([]string(nil), existing.GetSelectMessageDetailIds()...)
				terminal = existingTerminal
				if progress := existingProgressesByID[master.GetId()]; progress != nil &&
					isGodMessageTerminal(master, progress.LastMessageDetailId) &&
					progress.LastMessageDetailTime >= arrivedTime && progress.LastMessageDetailTime <= now {
					terminal = findGodMessageDetail(master, progress.LastMessageDetailId)
					finishedTime = progress.LastMessageDetailTime
				}
			}
		}
		message := &ptransaction.UserMessage{
			MessageId:              master.GetId(),
			MessageStatusType:      penum.MessageStatusType_MessageStatusType_Finished,
			ArrivedTime:            arrivedTime,
			SelectMessageDetailIds: selections,
		}
		messages = append(messages, godValue("messageId", master.GetId(), message))
		progresses = append(progresses, &umodel.UserMessageProgress{
			SUID:                  suid,
			MessageId:             master.GetId(),
			LastMessageDetailId:   terminal.GetMessageDetailId(),
			LastMessageDetailTime: finishedTime,
		})
	}
	if err := godUpsert(ctx, uname.UserMessage, suid, messages, true, counts); err != nil {
		return err
	}
	if err := opuser.UpsertManyUserMessageProgresses(ctx, progresses); err != nil {
		return err
	}
	counts[uname.UserMessageProgress] = len(progresses)
	if err := opuser.DeleteAllUserMessageSchedules(ctx, suid); err != nil {
		return err
	}
	counts[uname.UserMessageSchedule] = 0

	existingTelephones, err := opuser.FindAllUserTelephones(ctx, suid)
	if err != nil {
		return err
	}
	existingTelephonesByID := make(map[string]*ptransaction.UserTelephone, len(existingTelephones))
	for _, telephone := range existingTelephones {
		existingTelephonesByID[telephone.GetTelephoneId()] = telephone
	}
	telephones := make([]opuser.GodUpsert[*ptransaction.UserTelephone], 0, len(data.telephones))
	for _, master := range data.telephones {
		unlockedTime := now
		scheduledTime := now
		if existing := existingTelephonesByID[master.GetId()]; existing != nil {
			if existing.GetUnlockedTime() > 0 && existing.GetUnlockedTime() <= now {
				unlockedTime = existing.GetUnlockedTime()
			}
			if existing.GetScheduledTime() >= unlockedTime && existing.GetScheduledTime() <= now {
				scheduledTime = existing.GetScheduledTime()
			}
		}
		telephone := &ptransaction.UserTelephone{
			TelephoneId:         master.GetId(),
			UnlockedTime:        unlockedTime,
			ScheduledTime:       scheduledTime,
			TelephoneStatusType: penum.TelephoneStatusType_TelephoneStatusType_Finished,
		}
		telephones = append(telephones, godValue("telephoneId", master.GetId(), telephone))
	}
	if err := godUpsert(ctx, uname.UserTelephone, suid, telephones, true, counts); err != nil {
		return err
	}

	conditionByID := make(map[string]*pmaster.Condition, len(data.conditions))
	for _, condition := range data.conditions {
		conditionByID[condition.GetId()] = condition
	}
	storyIDs := make(map[string]struct{})
	for _, master := range data.messages {
		collectGodMessageStoryPrerequisites(master.GetUnlockConditionId(), conditionByID, storyIDs, make(map[string]struct{}))
	}
	readStoryIDs := make([]string, 0, len(storyIDs))
	for storyID := range storyIDs {
		readStoryIDs = append(readStoryIDs, storyID)
	}
	if err := opuser.MarkUserStoriesReadForGod(ctx, suid, readStoryIDs); err != nil {
		return err
	}
	return nil
}

func findGodMessageDetail(master *pmaster.Message, detailID string) *pmaster.MessageDetail {
	for _, detail := range master.GetDetails() {
		if detail.GetMessageDetailId() == detailID {
			return detail
		}
	}
	return nil
}

func isGodMessageTerminal(master *pmaster.Message, detailID string) bool {
	detail := findGodMessageDetail(master, detailID)
	return detail != nil && len(detail.GetNextMessageDetailIds()) == 0
}

func collectGodMessageStoryPrerequisites(
	conditionID string,
	conditions map[string]*pmaster.Condition,
	storyIDs map[string]struct{},
	visited map[string]struct{},
) {
	if conditionID == "" {
		return
	}
	if _, seen := visited[conditionID]; seen {
		return
	}
	visited[conditionID] = struct{}{}
	condition := conditions[conditionID]
	if condition == nil {
		return
	}
	for _, setting := range condition.GetSettings() {
		if readStory := setting.GetReadStory(); readStory.GetStoryId() != "" {
			storyIDs[readStory.GetStoryId()] = struct{}{}
		}
		if dependency := setting.GetSatisfyCondition(); dependency.GetConditionId() != "" {
			collectGodMessageStoryPrerequisites(dependency.GetConditionId(), conditions, storyIDs, visited)
		}
	}
}

func applyGodCompany(
	ctx context.Context,
	suid string,
	data *godMasterData,
	now int64,
	feature GodFeature,
	counts map[string]int,
) error {
	var maxCharacterPoint int64
	for _, level := range data.companyLevels {
		if level.GetRequiredPoint() > maxCharacterPoint {
			maxCharacterPoint = level.GetRequiredPoint()
		}
	}
	if maxCharacterPoint <= 0 {
		return fmt.Errorf("CompanyEnjoyCharacterLevel has no positive point threshold")
	}

	masterCharacters := make(map[string]*pmaster.Character, len(data.characters))
	for _, master := range data.characters {
		masterCharacters[master.GetId()] = master
	}
	characters, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		return err
	}
	var normalCount int64
	var totalEnjoyPoint int64
	for _, value := range characters {
		master := masterCharacters[value.GetCharacterId()]
		if master == nil || master.GetType() != penum.CharacterType_CharacterType_Normal {
			continue
		}
		if value.GetCompanyEnjoyPoint() < maxCharacterPoint {
			value.CompanyEnjoyPoint = maxCharacterPoint
		}
		if value.GetCompanyEnjoyPoint() > math.MaxInt64-totalEnjoyPoint {
			return fmt.Errorf("god company enjoy point total overflows")
		}
		totalEnjoyPoint += value.GetCompanyEnjoyPoint()
		normalCount++
	}
	if normalCount == 0 {
		return fmt.Errorf("god user owns no normal company character")
	}
	averageEnjoyPoint := totalEnjoyPoint / normalCount
	for _, value := range characters {
		master := masterCharacters[value.GetCharacterId()]
		if master == nil {
			continue
		}
		if master.GetType() == penum.CharacterType_CharacterType_Normal {
			if value.GetCompanyEnjoyPoint() < maxCharacterPoint {
				value.CompanyEnjoyPoint = maxCharacterPoint
			}
		} else {
			// Memories and collaboration idols inherit the joined normal-idol
			// average and do not contribute back to the company enjoy total.
			value.CompanyEnjoyPoint = averageEnjoyPoint
		}
		bonuses, err := godCompanyCharacterBonuses(data.companyLevels, value.GetCompanyEnjoyPoint())
		if err != nil {
			return fmt.Errorf("failed to derive Company bonuses for %q: %w", value.GetCharacterId(), err)
		}
		value.CompanyBonusParameterInfos = bonuses
		if err := opuser.UpsertUserCharacter(ctx, suid, value); err != nil {
			return err
		}
	}
	counts[uname.UserCharacter] = len(characters)

	company, err := opuser.FindUserCompany(ctx, suid)
	if err != nil {
		return err
	}
	company.EnjoyPoint = totalEnjoyPoint
	for _, level := range data.companyTrust {
		if level.GetRequiredPoint() > company.GetTrustPoint() {
			company.TrustPoint = level.GetRequiredPoint()
		}
	}
	var maxGradePoint int64
	for _, grade := range data.companyGrades {
		if grade.GetRequiredPoint() > maxGradePoint {
			maxGradePoint = grade.GetRequiredPoint()
		}
	}
	if company.GetEnjoyPoint() > math.MaxInt64-company.GetTrustPoint() {
		return fmt.Errorf("god company point total overflows")
	}
	nonSalaryPoint := company.GetEnjoyPoint() + company.GetTrustPoint()
	if nonSalaryPoint < maxGradePoint && company.GetSalaryPoint() < maxGradePoint-nonSalaryPoint {
		company.SalaryPoint = maxGradePoint - nonSalaryPoint
	}
	company.IsAlreadyTransitioned = true
	if err := opuser.UpsertUserCompany(ctx, suid, company); err != nil {
		return err
	}
	counts[uname.UserCompany] = 1

	rewardState := &umodel.UserCompanyRewardState{
		SUID:                            suid,
		ReceivedGradeIds:                make([]string, 0, len(data.companyGrades)),
		ReceivedTrustLevelKeys:          make([]int32, 0, len(data.companyTrust)),
		ReceivedEnjoyCharacterLevelKeys: make(map[string][]int32),
	}
	for _, grade := range data.companyGrades {
		if grade.GetId() != "" {
			rewardState.ReceivedGradeIds = append(rewardState.ReceivedGradeIds, grade.GetId())
		}
	}
	for _, level := range data.companyTrust {
		if level.GetLevelKey() > 0 {
			rewardState.ReceivedTrustLevelKeys = append(rewardState.ReceivedTrustLevelKeys, level.GetLevelKey())
		}
	}
	for _, setting := range data.companySettings {
		if setting.GetCharacterId() == "" || setting.GetLevelKey() <= 0 {
			continue
		}
		rewardState.ReceivedEnjoyCharacterLevelKeys[setting.GetCharacterId()] = append(
			rewardState.ReceivedEnjoyCharacterLevelKeys[setting.GetCharacterId()],
			setting.GetLevelKey(),
		)
	}
	if err := opuser.UpsertUserCompanyRewardState(ctx, rewardState); err != nil {
		return err
	}
	counts[uname.UserCompanyRewardState] = 1

	conditionByID := make(map[string]*pmaster.Condition, len(data.conditions))
	for _, condition := range data.conditions {
		conditionByID[condition.GetId()] = condition
	}
	godMissions := make([]opuser.GodUpsert[*ptransaction.UserMission], 0)
	for _, master := range data.missions {
		companyMission := master.GetCompanyTrustMissionCategoryType() != penum.CompanyTrustMissionCategoryType_CompanyTrustMissionCategoryType_Unknown ||
			master.GetCompanyEnjoyCharacterMissionCategoryType() != penum.CompanyEnjoyCharacterMissionCategoryType_CompanyEnjoyCharacterMissionCategoryType_Unknown ||
			master.GetCompanyEnjoyCharacterId() != ""
		reliabilityMission := master.GetMissionType() == penum.MissionType_MissionType_ReliabilityAchievement
		if reliabilityMission && feature.Bond != umodel.GodMessageFeatureAll {
			continue
		}
		if !companyMission && !reliabilityMission {
			continue
		}
		if !godCompanyConditionInTerm(conditionByID[master.GetViewConditionId()], master.GetViewConditionId(), now) {
			continue
		}
		var threshold int64
		for _, progress := range master.GetProgresses() {
			if progress.GetThreshold() > threshold {
				threshold = progress.GetThreshold()
			}
		}
		if threshold <= 0 {
			continue
		}
		godMissions = append(godMissions, godValue("missionId", master.GetId(), &ptransaction.UserMission{
			MissionId:        master.GetId(),
			CurrentThreshold: threshold,
			Progress:         threshold,
			StatusType:       penum.MissionStatusType_MissionStatusType_Received,
			LastUpdateTime:   now,
		}))
	}
	return godUpsert(ctx, uname.UserMission, suid, godMissions, true, counts)
}

func godCompanyConditionInTerm(condition *pmaster.Condition, conditionID string, now int64) bool {
	if conditionID == "" {
		return true
	}
	if condition == nil {
		return false
	}
	for _, setting := range condition.GetSettings() {
		term := setting.GetTerm()
		if term == nil ||
			(term.GetNowAfter() <= 0 || now >= term.GetNowAfter()) &&
				(term.GetNowBefore() <= 0 || now <= term.GetNowBefore()) {
			return true
		}
	}
	return false
}

func godCompanyCharacterBonuses(
	levels []*pmaster.CompanyEnjoyCharacterLevel,
	point int64,
) ([]*ptransaction.CompanyBonusParameterInfo, error) {
	parameterTypes := []penum.ParameterType{
		penum.ParameterType_ParameterType_Dance,
		penum.ParameterType_ParameterType_Vocal,
		penum.ParameterType_ParameterType_Visual,
		penum.ParameterType_ParameterType_Stamina,
		penum.ParameterType_ParameterType_Mental,
		penum.ParameterType_ParameterType_Technique,
	}
	bonuses := make(map[penum.ParameterType]int64, len(parameterTypes))
	for _, level := range levels {
		parameterType := level.GetBonusParameterType()
		if level.GetRequiredPoint() > point || parameterType == penum.ParameterType_ParameterType_Unknown {
			continue
		}
		increment := int64(level.GetBonusParameterUpPermil())
		if increment < 0 || bonuses[parameterType] > math.MaxInt64-increment {
			return nil, fmt.Errorf("bonus parameter overflows")
		}
		bonuses[parameterType] += increment
	}
	result := make([]*ptransaction.CompanyBonusParameterInfo, 0, len(parameterTypes))
	for _, parameterType := range parameterTypes {
		result = append(result, &ptransaction.CompanyBonusParameterInfo{
			BonusParameterType:     parameterType,
			BonusParameterUpPermil: bonuses[parameterType],
		})
	}
	return result, nil
}

func applyGodProgress(
	ctx context.Context,
	suid string,
	data *godMasterData,
	now int64,
	counts map[string]int,
) error {
	maxRanks := make(map[penum.ParameterType]int32, len(data.staffRanks))
	for _, rank := range data.staffRanks {
		maxRanks[rank.ParameterType] = rank.Rank
	}
	staffs := make([]opuser.GodUpsert[*ptransaction.UserStaff], 0, len(data.staffLevels))
	for _, level := range data.staffLevels {
		staffs = append(staffs, godValue("parameterType", level.ParameterType, &ptransaction.UserStaff{
			ParameterType: level.ParameterType, Level: level.Level,
			LevelLimitBreakRank: maxRanks[level.ParameterType],
		}))
	}
	if err := godUpsert(ctx, uname.UserStaff, suid, staffs, true, counts); err != nil {
		return err
	}

	areas := make([]*ptransaction.UserArea, 0, len(data.areas))
	for _, master := range data.areas {
		areas = append(areas, &ptransaction.UserArea{
			AreaId: master.Id, ClearQuestCount: int32(len(master.QuestIds)), LastClearedTime: now,
		})
	}
	if err := opuser.UnlockUserAreasForGod(ctx, suid, areas); err != nil {
		return err
	}
	counts[uname.UserArea] = len(areas)

	quests := make([]*ptransaction.UserQuest, 0, len(data.quests))
	for _, master := range data.quests {
		quests = append(quests, &ptransaction.UserQuest{
			QuestId: master.Id, LastClearedTime: now, HighestScore: master.ClearScore,
			HighestRank: 1, HighestScoreRank: penum.ResultRankType_ResultRankType_S,
		})
	}
	if err := opuser.UnlockUserQuestsForGod(ctx, suid, quests); err != nil {
		return err
	}
	counts[uname.UserQuest] = len(quests)

	tutorials := make([]opuser.GodUpsert[*ptransaction.UserTutorial], 0, len(data.tutorials))
	for _, value := range data.tutorials {
		tutorials = append(tutorials, godValue("tutorialType", value.TutorialType, value))
	}
	if err := godUpsert(ctx, uname.UserTutorial, suid, tutorials, true, counts); err != nil {
		return err
	}

	eventStories := make([]opuser.GodUpsert[*papi.EventStoryInfo], 0, len(data.eventStories))
	for _, master := range data.eventStories {
		episodes := make([]*papi.EventStoryEpisodeInfo, 0, len(master.Episodes))
		for _, episode := range master.Episodes {
			episodes = append(episodes, &papi.EventStoryEpisodeInfo{Episode: episode.Episode, Unlocked: true})
		}
		eventStories = append(eventStories, godValue("id", master.Id, &papi.EventStoryInfo{Id: master.Id, Episodes: episodes}))
	}
	if err := godUpsert(ctx, uname.UserEventStoryInfo, suid, eventStories, true, counts); err != nil {
		return err
	}

	extraStoriesByPart := make(map[string][]*papi.ExtraStoryInfo)
	for _, master := range data.extraStories {
		episodes := make([]*papi.ExtraStoryEpisodeInfo, 0, len(master.Episodes))
		for _, episode := range master.Episodes {
			episodes = append(episodes, &papi.ExtraStoryEpisodeInfo{Episode: episode.Episode, Unlocked: true})
		}
		extraStoriesByPart[master.ExtraStoryPartId] = append(extraStoriesByPart[master.ExtraStoryPartId], &papi.ExtraStoryInfo{
			Id: master.Id, Unlocked: true, Episodes: episodes,
		})
	}
	extraParts := make([]opuser.GodUpsert[*papi.ExtraStoryPartInfo], 0, len(data.extraStoryParts))
	for _, master := range data.extraStoryParts {
		extraParts = append(extraParts, godValue("id", master.Id, &papi.ExtraStoryPartInfo{
			Id: master.Id, Unlocked: true, Stories: extraStoriesByPart[master.Id],
		}))
	}
	if err := godUpsert(ctx, uname.UserExtraStoryPartInfo, suid, extraParts, true, counts); err != nil {
		return err
	}

	// Shooting catalogs are a projection of current master data. Rebuild them so
	// rows from older master versions do not remain visible after god mode.
	if err := opuser.DeleteAllUserPhotoActivities(ctx, suid); err != nil {
		return err
	}
	if err := opuser.DeleteAllUserPhotoExpressions(ctx, suid); err != nil {
		return err
	}
	if err := opuser.DeleteAllUserPhotoMusics(ctx, suid); err != nil {
		return err
	}
	if err := opuser.DeleteAllUserPhotoStages(ctx, suid); err != nil {
		return err
	}

	activities := make([]opuser.GodUpsert[*papi.PhotoActivity], 0, len(data.photoActivities))
	for _, master := range data.photoActivities {
		activities = append(activities, godValue("photoActivityId", master.Id, &papi.PhotoActivity{PhotoActivityId: master.Id, IsUnlocked: true}))
	}
	if err := godUpsert(ctx, uname.UserPhotoActivity, suid, activities, true, counts); err != nil {
		return err
	}
	expressions := make([]opuser.GodUpsert[*papi.PhotoExpression], 0, len(data.photoExpressions))
	for _, master := range data.photoExpressions {
		expressions = append(expressions, godValue("photoExpressionId", master.Id, &papi.PhotoExpression{PhotoExpressionId: master.Id, IsUnlocked: true}))
	}
	if err := godUpsert(ctx, uname.UserPhotoExpression, suid, expressions, true, counts); err != nil {
		return err
	}
	poses := make([]opuser.GodUpsert[*ptransaction.UserPhotoPose], 0, len(data.photoPoses))
	for _, master := range data.photoPoses {
		poses = append(poses, godValue("photoPoseId", master.Id, &ptransaction.UserPhotoPose{PhotoPoseId: master.Id}))
	}
	if err := godUpsert(ctx, uname.UserPhotoPose, suid, poses, false, counts); err != nil {
		return err
	}
	photoMusics := make([]opuser.GodUpsert[*papi.PhotoMusic], 0, len(data.photoMusics))
	for _, master := range data.photoMusics {
		photoMusics = append(photoMusics, godValue("musicId", master.Id, &papi.PhotoMusic{MusicId: master.Id, IsUnlocked: true}))
	}
	if err := godUpsert(ctx, uname.UserPhotoMusic, suid, photoMusics, true, counts); err != nil {
		return err
	}
	excursionGazes := make([]opuser.GodUpsert[*ptransaction.UserExcursionGazeReaction], 0, len(data.excursionGazes))
	for _, master := range data.excursionGazes {
		reaction := &ptransaction.UserExcursionGazeReaction{
			CharacterId:        master.CharacterId,
			ExcursionPlaceId:   master.ExcursionPlaceId,
			GazeReactionNumber: int64(master.Number),
		}
		excursionGazes = append(excursionGazes, opuser.GodUpsert[*ptransaction.UserExcursionGazeReaction]{
			Key: bson.M{
				"characterId":        reaction.CharacterId,
				"excursionPlaceId":   reaction.ExcursionPlaceId,
				"gazeReactionNumber": reaction.GazeReactionNumber,
			},
			Data: reaction,
		})
	}
	if err := godUpsert(ctx, uname.UserExcursionGazeReaction, suid, excursionGazes, false, counts); err != nil {
		return err
	}
	stages := make([]opuser.GodUpsert[*papi.PhotoStage], 0, len(data.photoStages))
	for _, master := range data.photoStages {
		stages = append(stages, godValue("stageId", master.Id, &papi.PhotoStage{StageId: master.Id, IsUnlocked: true}))
	}
	return godUpsert(ctx, uname.UserPhotoStage, suid, stages, true, counts)
}

func applyGodResources(
	ctx context.Context,
	suid string,
	data *godMasterData,
	counts map[string]int,
) error {
	accessories := make([]opuser.GodUpsert[*ptransaction.UserAccessory], 0, len(data.accessories))
	for _, master := range data.accessories {
		accessories = append(accessories, godValue("accessoryId", master.Id, &ptransaction.UserAccessory{
			AccessoryId: master.Id, Amount: GodMinimumResourceAmount,
		}))
	}
	if err := godUpsert(ctx, uname.UserAccessory, suid, accessories, false, counts); err != nil {
		return err
	}
	items := make([]opuser.GodUpsert[*ptransaction.UserItem], 0, len(data.items))
	excludedItemIds := make([]string, 0)
	for _, master := range data.items {
		if master.GetType() == penum.ItemType_ItemType_GachaTicket {
			excludedItemIds = append(excludedItemIds, master.GetId())
			continue
		}
		value := &ptransaction.UserItem{ItemId: master.Id, Amount: GodMinimumResourceAmount}
		items = append(items, opuser.GodUpsert[*ptransaction.UserItem]{
			Key: bson.M{"itemId": master.Id, "expiredTime": int64(0)}, Data: value,
		})
	}
	if err := godUpsert(ctx, uname.UserItem, suid, items, false, counts); err != nil {
		return err
	}
	if err := opuser.DeleteGodGrantedUserItems(ctx, suid, excludedItemIds, GodMinimumResourceAmount); err != nil {
		return err
	}
	recipes := make([]opuser.GodUpsert[*ptransaction.UserPhotoRecipe], 0)
	for _, master := range data.photoRecipes {
		// user recipe capacity is extremely limited, populates high rarity recipes only
		if strings.HasPrefix(master.Id, "photo-recipe-5-rare-5-") || strings.HasPrefix(master.Id, "photo-recipe-5-rare-4-") {
			recipes = append(recipes, godValue("photoRecipeId", master.Id, &ptransaction.UserPhotoRecipe{
				PhotoRecipeId: master.Id, Amount: 1,
			}))
		}
	}
	if err := godUpsert(ctx, uname.UserPhotoRecipe, suid, recipes, false, counts); err != nil {
		return err
	}
	points := make([]opuser.GodUpsert[*ptransaction.UserPoint], 0, 7)
	for pointType := penum.PointType_PointType_Gold; pointType <= penum.PointType_PointType_ActivatePhotoShootingStoryTicket; pointType++ {
		points = append(points, godValue("pointType", pointType, &ptransaction.UserPoint{PointType: pointType, Amount: GodMinimumPointAmount}))
	}
	if err := godUpsert(ctx, uname.UserPoint, suid, points, false, counts); err != nil {
		return err
	}
	if err := opuser.RaiseUserResourcesForGod(ctx, suid, GodMinimumResourceAmount, excludedItemIds); err != nil {
		return err
	}
	if err := opuser.RaiseUserPointsForGod(ctx, suid, GodMinimumPointAmount); err != nil {
		return err
	}
	if err := opuser.RaiseUserBalanceForGod(ctx, suid, GodMinimumBalanceAmount); err != nil {
		return err
	}
	counts[uname.UserBalance] = 1

	shops, err := opsolis.FindAllShopCatalogEntries(ctx)
	if err != nil {
		return err
	}
	shopPurchase, err := opuser.FindUserShopPurchase(ctx, suid)
	if errors.Is(err, mongo.ErrNoDocuments) {
		shopPurchase = &umodel.UserShopPurchase{SUID: suid}
	} else if err != nil {
		return err
	}
	if shopPurchase.PurchaseCounts == nil {
		shopPurchase.PurchaseCounts = map[string]int32{}
	}
	if shopPurchase.ResetTimes == nil {
		shopPurchase.ResetTimes = map[string]int64{}
	}
	purchasedItemIds := append([]string(nil), shopPurchase.PurchasedItemIds...)
	for _, shop := range shops {
		for _, item := range shop.Items {
			if item != nil {
				purchasedItemIds = append(purchasedItemIds, item.GetId())
			}
		}
	}
	shopPurchase.PurchasedItemIds = uniqueGodStrings(purchasedItemIds)
	if err := opuser.UpsertUserShopPurchase(ctx, shopPurchase); err != nil {
		return err
	}
	counts[uname.UserShopPurchase] = len(shopPurchase.PurchasedItemIds)

	currentShops, err := opuser.FindUserShopIds(ctx, suid)
	if err != nil {
		return err
	}
	showcaseCatalogIds := make(map[string]struct{}, len(shops))
	showcaseShopIds := make([]string, 0, len(shops))
	for _, shop := range shops {
		if shop.Info == nil {
			continue
		}
		switch shop.Info.Type {
		case penum.ShopType_ShopType_ShowcaseToyShop,
			penum.ShopType_ShopType_DefaultShowcaseToyShop,
			penum.ShopType_ShopType_OnlyShowcaseShop:
			showcaseCatalogIds[shop.ShopId] = struct{}{}
			showcaseShopIds = append(showcaseShopIds, shop.ShopId)
		}
	}
	shopIds := make([]string, 0, len(currentShops.ShopIds)+len(shops))
	for _, shopId := range currentShops.ShopIds {
		if _, isShowcase := showcaseCatalogIds[shopId]; !isShowcase {
			shopIds = append(shopIds, shopId)
		}
	}
	for _, shop := range shops {
		if shop.Info == nil {
			continue
		}
		if _, isShowcase := showcaseCatalogIds[shop.ShopId]; !isShowcase {
			shopIds = append(shopIds, shop.ShopId)
		}
	}
	shopIds = uniqueGodStrings(shopIds)
	if err := opuser.UpsertUserShopId(ctx, &umodel.UserShopId{SUID: suid, ShopIds: shopIds}); err != nil {
		return err
	}
	counts[uname.UserShopId] = len(shopIds)
	currentShowcaseShops, err := opuser.FindUserShowcaseToyShopIds(ctx, suid)
	if err != nil {
		return err
	}
	showcaseShopIds = uniqueGodStrings(append(currentShowcaseShops.ShopIds, showcaseShopIds...))
	if err := opuser.UpsertUserShowcaseToyShopId(ctx, &umodel.UserShowcaseToyShopId{
		SUID: suid, ShopIds: showcaseShopIds,
	}); err != nil {
		return err
	}
	counts[uname.UserShowcaseToyShopId] = len(showcaseShopIds)

	booths, err := opsolis.FindAllExchangeBooths(ctx)
	if err != nil {
		return err
	}
	currentBooths, err := opuser.FindUserExchangeBooth(ctx, suid)
	if err != nil {
		return err
	}
	boothIds := append([]string(nil), currentBooths.BoothIds...)
	for _, booth := range booths {
		boothIds = append(boothIds, booth.Id)
	}
	boothIds = uniqueGodStrings(boothIds)
	if err := opuser.UpsertUserExchangeBooth(ctx, &umodel.UserExchangeBooth{SUID: suid, BoothIds: boothIds}); err != nil {
		return err
	}
	counts[uname.UserExchangeBooth] = len(boothIds)
	return nil
}

func mainGodStoryIds(parts []*pmaster.StoryPart) []string {
	storyIds := make([]string, 0)
	for _, part := range parts {
		if part.GetType() != penum.StoryPartType_StoryPartType_Main {
			continue
		}
		for _, chapter := range part.GetChapters() {
			for _, episode := range chapter.GetEpisodes() {
				storyIds = append(storyIds, episode.GetStoryId())
			}
		}
	}
	return uniqueGodStrings(storyIds)
}

func uniqueGodStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func godValue[T interface{ ProtoReflect() protoreflect.Message }](key string, value any, data T) opuser.GodUpsert[T] {
	return opuser.GodUpsert[T]{Key: bson.M{key: value}, Data: data}
}

func godUpsert[T interface{ ProtoReflect() protoreflect.Message }](
	ctx context.Context,
	collection, suid string,
	values []opuser.GodUpsert[T],
	overwrite bool,
	counts map[string]int,
) error {
	if err := opuser.UpsertManyForGod(ctx, collection, suid, values, overwrite); err != nil {
		return err
	}
	counts[collection] = len(values)
	return nil
}
