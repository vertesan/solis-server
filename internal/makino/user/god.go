package user

import (
	"context"
	"fmt"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
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
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	GodMinimumResourceAmount int64 = 1_000_000
	GodMinimumBalanceAmount  int32 = 999_999
	GodMinimumPointAmount    int64 = 100_000_000_000
)

type GodResult struct {
	ServerUserId string
	Collections  map[string]int
}

type godMasterData struct {
	managerLevel     *pmaster.ManagerLevel
	cardSupport      *pmaster.CardSupportRelease
	setting          *pmaster.Setting
	accessories      []*pmaster.Accessory
	activityFans     []*pmaster.ActivityFanEvent
	activityPromos   []*pmaster.ActivityPromotion
	activityRefresh  []*pmaster.ActivityRefresh
	areas            []*pmaster.Area
	cards            []*pmaster.Card
	characters       []*pmaster.Character
	costumes         []*pmaster.Costume
	decorations      []*pmaster.Decoration
	emblems          []*pmaster.Emblem
	eventStories     []*pmaster.EventStory
	extraStories     []*pmaster.ExtraStory
	extraStoryParts  []*pmaster.ExtraStoryPart
	hairs            []*pmaster.Hair
	homeTalks        []*pmaster.HomeTalk
	items            []*pmaster.Item
	loveHomeActions  []*pmaster.LoveHomeAction
	loveLevels       []*pmaster.LoveLevel
	messages         []*pmaster.Message
	musics           []*pmaster.Music
	photoActivities  []*pmaster.PhotoActivity
	photoExpressions []*pmaster.PhotoExpression
	photoPoses       []*pmaster.PhotoPose
	photoRecipes     []*pmaster.PhotoRecipe
	quests           []*pmaster.Quest
	showcaseMusics   []*pmaster.ShowcaseMusic
	showcaseToys     []*pmaster.ShowcaseToy
	staffLevels      []*pmaster.StaffLevel
	staffRanks       []*pmaster.StaffLevelLimitBreakRank
	stages           []*pmaster.Stage
	stories          []*pmaster.Story
	telephones       []*pmaster.Telephone
	tutorials        []*ptransaction.UserTutorial
	userCards        []*ptransaction.UserCard
	userCharacters   []*ptransaction.UserCharacter
	userCostumes     []*ptransaction.UserCostume
}

func GodUser(ctx context.Context, publicUserId, serverUserId string) (*GodResult, error) {
	if (publicUserId == "") == (serverUserId == "") {
		return nil, fmt.Errorf("exactly one of publicUserId or serverUserId is required")
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
		if err := applyGodMasterData(ctx, auth.ServerUserId, masters, result.Collections); err != nil {
			return nil, err
		}
		return nil, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to unlock user: %w", err)
	}
	return result, nil
}

func findGodUserAuth(ctx context.Context, publicUserId, serverUserId string) (*umodel.UserAuth, error) {
	if publicUserId != "" {
		return opuser.GetUserAuthByPublicId(ctx, publicUserId)
	}
	return opuser.GetUserAuthByServerId(ctx, serverUserId)
}

func loadGodMasterData(ctx context.Context) (*godMasterData, error) {
	data := &godMasterData{}
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
	run("PhotoPose", func() (err error) {
		data.photoPoses, err = opmaster.FindAllPhotoPoses(groupCtx)
		return err
	})
	run("PhotoRecipe", func() (err error) {
		data.photoRecipes, err = opmaster.FindAllPhotoRecipes(groupCtx)
		return err
	})
	run("Quest", func() (err error) {
		data.quests, err = opmaster.FindAllQuests(groupCtx)
		return err
	})
	run("Stage", func() (err error) {
		data.stages, err = opmaster.FindAllStages(groupCtx)
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
	if err := opuser.UpsertUser(ctx, suid, userData); err != nil {
		return err
	}
	counts[uname.User] = 1

	if err := applyGodOwnership(ctx, suid, data, now, counts); err != nil {
		return err
	}
	if err := applyGodProgress(ctx, suid, data, now, counts); err != nil {
		return err
	}
	if err := applyGodFeatureGates(ctx, suid, data, counts); err != nil {
		return err
	}
	if err := applyGodResources(ctx, suid, data, counts); err != nil {
		return err
	}
	return nil
}

func applyGodFeatureGates(
	ctx context.Context,
	suid string,
	data *godMasterData,
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
	for _, master := range data.activityFans {
		value := fansById[master.Id]
		if value == nil {
			value = &papi.FanEvent{Id: master.Id}
			fanInfo.FanEvents = append(fanInfo.FanEvents, value)
		}
		value.Name = master.Name
		value.Type = master.Type
		value.ViewConditionId = master.ViewConditionId
		value.UnlockConditionId = master.UnlockConditionId
		value.IsUnlocked = true
		var highestLevel *pmaster.ActivityFanEventLevel
		for _, level := range master.Levels {
			if highestLevel == nil || level.Level > highestLevel.Level {
				highestLevel = level
			}
		}
		if highestLevel != nil {
			value.Level = highestLevel.Level
			value.SubName = highestLevel.Name
		}
		value.NextLevelRequiredExp = 0
		value.NextLevelUnlockConditionId = ""
		value.IsNextLevelUnlocked = false
	}
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
	for _, master := range data.activityPromos {
		value := promotionsById[master.Id]
		if value == nil {
			value = &papi.Promotion{Id: master.Id}
			promotionInfo.Promotions = append(promotionInfo.Promotions, value)
		}
		value.Name = master.Name
		value.ViewConditionId = master.ViewConditionId
		value.UnlockConditionId = master.UnlockConditionId
		value.IsUnlocked = true
		var highestLevel *pmaster.ActivityPromotionLevel
		for _, level := range master.Levels {
			if highestLevel == nil || level.Level > highestLevel.Level {
				highestLevel = level
			}
		}
		if highestLevel != nil {
			value.Level = highestLevel.Level
			value.SubName = highestLevel.Name
		}
		value.NextLevelRequiredExp = 0
		value.NextLevelUnlockConditionId = ""
		value.IsNextLevelUnlocked = false
	}
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
	for _, master := range data.activityRefresh {
		value := refreshesById[master.Id]
		if value == nil {
			value = &papi.Refresh{Id: master.Id}
			refreshInfo.Refreshes = append(refreshInfo.Refreshes, value)
		}
		value.Name = master.Name
		value.ViewConditionId = master.ViewConditionId
		value.UnlockConditionId = master.UnlockConditionId
		value.IsUnlocked = true
		levelsByNumber := make(map[int32]*papi.RefreshLevel, len(value.Levels))
		for _, level := range value.Levels {
			levelsByNumber[level.Level] = level
		}
		var highestLevel int32
		for _, masterLevel := range master.Levels {
			if masterLevel.Level > highestLevel {
				highestLevel = masterLevel.Level
			}
			level := levelsByNumber[masterLevel.Level]
			if level == nil {
				level = &papi.RefreshLevel{Level: masterLevel.Level}
				value.Levels = append(value.Levels, level)
			}
			level.Name = masterLevel.Name
			level.ViewConditionId = masterLevel.ViewConditionId
			level.UnlockConditionId = masterLevel.UnlockConditionId
			level.IsUnlocked = true
		}
		if highestLevel > 0 {
			value.Level = highestLevel
		}
		value.NextLevelRequiredExp = 0
	}
	if err := opuser.UpsertUserRefreshInfo(ctx, suid, refreshInfo); err != nil {
		return err
	}
	counts[uname.UserRefreshInfo] = len(refreshInfo.Refreshes)

	current, err := opuser.FindUserCurrentQuestArea(ctx, suid)
	if err != nil {
		return err
	}
	mainArea, towerArea := furthestGodAreas(data.areas)
	if mainArea != nil {
		current.CurrentMainAreaId = mainArea.Id
		lesson, err := opuser.FindUserLessonProgress(ctx, suid)
		if err != nil {
			return err
		}
		lesson.CurrentAreaId = mainArea.Id
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
	for _, value := range data.userCharacters {
		characters = append(characters, godValue("characterId", value.CharacterId, value))
	}
	if err := godUpsert(ctx, uname.UserCharacter, suid, characters, false, counts); err != nil {
		return err
	}

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

	return applyGodMediaOwnership(ctx, suid, data, now, counts)
}

func applyGodMediaOwnership(
	ctx context.Context,
	suid string,
	data *godMasterData,
	now int64,
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

	telephones := make([]opuser.GodUpsert[*ptransaction.UserTelephone], 0, len(data.telephones))
	for _, master := range data.telephones {
		telephones = append(telephones, godValue("telephoneId", master.Id, &ptransaction.UserTelephone{
			TelephoneId: master.Id, UnlockedTime: now,
			TelephoneStatusType: penum.TelephoneStatusType_TelephoneStatusType_Finished,
		}))
	}
	if err := godUpsert(ctx, uname.UserTelephone, suid, telephones, false, counts); err != nil {
		return err
	}

	messages := make([]opuser.GodUpsert[*ptransaction.UserMessage], 0, len(data.messages))
	for _, master := range data.messages {
		messages = append(messages, godValue("messageId", master.Id, &ptransaction.UserMessage{
			MessageId: master.Id, MessageStatusType: penum.MessageStatusType_MessageStatusType_Finished,
			ArrivedTime: now, SelectMessageDetailIds: []string{},
		}))
	}
	if err := godUpsert(ctx, uname.UserMessage, suid, messages, false, counts); err != nil {
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
	photoMusics := make([]opuser.GodUpsert[*papi.PhotoMusic], 0, len(data.musics))
	for _, master := range data.musics {
		if master.Is3DAvailable {
			photoMusics = append(photoMusics, godValue("musicId", master.Id, &papi.PhotoMusic{MusicId: master.Id, IsUnlocked: true}))
		}
	}
	if err := godUpsert(ctx, uname.UserPhotoMusic, suid, photoMusics, true, counts); err != nil {
		return err
	}
	stages := make([]opuser.GodUpsert[*papi.PhotoStage], 0, len(data.stages))
	for _, master := range data.stages {
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
	for _, master := range data.items {
		value := &ptransaction.UserItem{ItemId: master.Id, Amount: GodMinimumResourceAmount}
		items = append(items, opuser.GodUpsert[*ptransaction.UserItem]{
			Key: bson.M{"itemId": master.Id, "expiredTime": int64(0)}, Data: value,
		})
	}
	if err := godUpsert(ctx, uname.UserItem, suid, items, false, counts); err != nil {
		return err
	}
	recipes := make([]opuser.GodUpsert[*ptransaction.UserPhotoRecipe], 0, len(data.photoRecipes))
	for _, master := range data.photoRecipes {
		recipes = append(recipes, godValue("photoRecipeId", master.Id, &ptransaction.UserPhotoRecipe{
			PhotoRecipeId: master.Id, Amount: GodMinimumResourceAmount,
		}))
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
	if err := opuser.RaiseUserResourcesForGod(ctx, suid, GodMinimumResourceAmount); err != nil {
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
