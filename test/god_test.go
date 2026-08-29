package test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	messagepath "github.com/vertesan/solis-server/internal/makino/message"
	makinouser "github.com/vertesan/solis-server/internal/makino/user"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/http/admin"
	"github.com/vertesan/solis-server/internal/utils/config"

	"github.com/gofiber/fiber/v3/client"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func callUserGod(t *testing.T, body *admin.UserGodRequest) *admin.UserGodResponse {
	t.Helper()
	response, err := testConfig.HttpClient.Post(
		fmt.Sprintf("%s/admin/user/god", testConfig.HttpUrl),
		client.Config{Header: getAdminAuthHeader(), Body: body},
	)
	if err != nil {
		t.Fatalf("admin user god failed: %v", err)
	}
	if response.StatusCode() != 200 {
		t.Fatalf("admin user god returned status %d: %s", response.StatusCode(), response.Body())
	}
	result := &admin.UserGodResponse{}
	if err := response.JSON(result); err != nil {
		t.Fatalf("failed to decode admin user god response: %v", err)
	}
	return result
}

func promoteTestUser(t *testing.T) {
	t.Helper()
	response := callUserGod(t, &admin.UserGodRequest{ServerUserId: testConfig.TestUserSuid})
	if !response.Ok || response.ServerUserId != testConfig.TestUserSuid {
		t.Fatalf("unexpected admin user god response: %v", response)
	}
}

func TestUserGod(t *testing.T) {
	setupTestUser(t)

	first := callUserGod(t, &admin.UserGodRequest{PublicUserId: testConfig.TestUserPublicId})

	ctx := context.Background()
	settingID, err := config.GetGameSettingId()
	if err != nil {
		t.Fatal(err)
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		t.Fatal(err)
	}
	userAfterFirstGod, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if userAfterFirstGod.GetPhotoLimitExtendedCount() != setting.GetPhotoLimitExtendLimit() {
		t.Fatalf("photo limit extension count = %d, want master limit %d",
			userAfterFirstGod.GetPhotoLimitExtendedCount(), setting.GetPhotoLimitExtendLimit())
	}
	preservedPhotoLimitExtendedCount := setting.GetPhotoLimitExtendLimit() + 1
	userAfterFirstGod.PhotoLimitExtendedCount = preservedPhotoLimitExtendedCount
	if err := opuser.UpsertUser(ctx, testConfig.TestUserSuid, userAfterFirstGod); err != nil {
		t.Fatal(err)
	}
	assertGodGrantedNoGachaTickets(t, ctx)
	lovesBeforeSecondCall, err := opuser.FindAllUserLoves(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(lovesBeforeSecondCall) == 0 {
		t.Fatal("god mode did not create master-derived love progression")
	}
	loveToPreserve := proto.Clone(lovesBeforeSecondCall[0]).(*ptransaction.UserLove)
	loveToPreserve.HiddenFavorability++
	loveToPreserve.IsReadingStory = true
	loveToPreserve.UnlockedLoveStoryEndIDs = append(loveToPreserve.UnlockedLoveStoryEndIDs, "test-preserved-love-story-end")
	if err := opuser.UpsertUserLove(ctx, testConfig.TestUserSuid, loveToPreserve); err != nil {
		t.Fatal(err)
	}
	masterMessagesForHistory, err := opmaster.FindAllMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var messageHistoryToPreserve *ptransaction.UserMessage
	var progressToPreserve *umodel.UserMessageProgress
	for _, master := range masterMessagesForHistory {
		firstSelections, firstTerminal, err := messagepath.FirstPath(master)
		if err != nil {
			t.Fatal(err)
		}
		alternateSelections, ok := findAlternateMessagePath(master, firstTerminal.GetMessageDetailId(), firstSelections)
		if !ok {
			continue
		}
		alternateTerminal, err := messagepath.ExistingHistoryPath(master, alternateSelections)
		if err != nil {
			t.Fatal(err)
		}
		arrivedTime := time.Now().Add(-2 * time.Minute).UnixMilli()
		finishedTime := time.Now().Add(-time.Minute).UnixMilli()
		messageHistoryToPreserve = &ptransaction.UserMessage{
			MessageId:              master.GetId(),
			MessageStatusType:      penum.MessageStatusType_MessageStatusType_Finished,
			ArrivedTime:            arrivedTime,
			SelectMessageDetailIds: alternateSelections,
		}
		progressToPreserve = &umodel.UserMessageProgress{
			SUID:                  testConfig.TestUserSuid,
			MessageId:             master.GetId(),
			LastMessageDetailId:   alternateTerminal.GetMessageDetailId(),
			LastMessageDetailTime: finishedTime,
		}
		if err := opuser.UpsertUserMessage(ctx, testConfig.TestUserSuid, messageHistoryToPreserve); err != nil {
			t.Fatal(err)
		}
		if err := opuser.UpsertUserMessageProgress(ctx, progressToPreserve); err != nil {
			t.Fatal(err)
		}
		break
	}
	if messageHistoryToPreserve == nil {
		t.Fatal("master data contains no alternate message path to test god history preservation")
	}
	telephonesBeforeSecondCall, err := opuser.FindAllUserTelephones(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(telephonesBeforeSecondCall) == 0 {
		t.Fatal("god mode did not create telephone history")
	}
	telephoneToComplete := proto.Clone(telephonesBeforeSecondCall[0]).(*ptransaction.UserTelephone)
	telephoneToComplete.TelephoneStatusType = penum.TelephoneStatusType_TelephoneStatusType_Scheduled
	telephoneToComplete.UnlockedTime = time.Now().Add(-time.Minute).UnixMilli()
	telephoneToComplete.ScheduledTime = time.Now().Add(time.Hour).UnixMilli()
	if err := opuser.UpsertUserTelephone(ctx, testConfig.TestUserSuid, telephoneToComplete); err != nil {
		t.Fatal(err)
	}
	ticketItems, err := opmaster.FindAllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ticketItemIds := make([]string, 0, 2)
	for _, item := range ticketItems {
		if item.GetType() == penum.ItemType_ItemType_GachaTicket {
			ticketItemIds = append(ticketItemIds, item.GetId())
			if len(ticketItemIds) == 2 {
				break
			}
		}
	}
	if len(ticketItemIds) < 2 {
		t.Fatal("master data contains fewer than two gacha ticket items")
	}
	legitimateTicketExpiration := time.Now().Add(24 * time.Hour).UnixMilli()
	if err := opuser.UpsertManyUserItems(ctx, testConfig.TestUserSuid, []*ptransaction.UserItem{
		{ItemId: ticketItemIds[0], Amount: makinouser.GodMinimumResourceAmount},
		{ItemId: ticketItemIds[1], Amount: 1},
		{ItemId: ticketItemIds[1], Amount: makinouser.GodMinimumResourceAmount, ExpiredTime: legitimateTicketExpiration},
	}); err != nil {
		t.Fatal(err)
	}

	secondGodStarted := time.Now().UnixMilli()
	second := callUserGod(t, &admin.UserGodRequest{ServerUserId: testConfig.TestUserSuid})
	secondGodFinished := time.Now().UnixMilli()
	if !first.Ok || !second.Ok || first.ServerUserId != testConfig.TestUserSuid || second.ServerUserId != testConfig.TestUserSuid {
		t.Fatalf("unexpected admin user god responses: first=%v second=%v", first, second)
	}

	lovesAfterSecondCall, err := opuser.FindAllUserLoves(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	var preservedLoveFound bool
	for _, love := range lovesAfterSecondCall {
		if love.LoveId != loveToPreserve.LoveId {
			continue
		}
		preservedStoryEndFound := false
		for _, storyEndID := range love.UnlockedLoveStoryEndIDs {
			if storyEndID == "test-preserved-love-story-end" {
				preservedStoryEndFound = true
				break
			}
		}
		preservedLoveFound = love.HiddenFavorability == loveToPreserve.HiddenFavorability && love.IsReadingStory && preservedStoryEndFound
		break
	}
	if !preservedLoveFound {
		t.Fatalf("god mode did not preserve existing love state: before=%v after=%v", loveToPreserve, lovesAfterSecondCall)
	}
	preservedMessage, err := opuser.FindUserMessage(ctx, testConfig.TestUserSuid, messageHistoryToPreserve.GetMessageId())
	if err != nil || !proto.Equal(preservedMessage, messageHistoryToPreserve) {
		t.Fatalf("god mode changed valid message history: before=%v after=%v err=%v", messageHistoryToPreserve, preservedMessage, err)
	}
	preservedProgress, err := opuser.FindUserMessageProgress(ctx, testConfig.TestUserSuid, progressToPreserve.MessageId)
	if err != nil || *preservedProgress != *progressToPreserve {
		t.Fatalf("god mode changed valid message progress: before=%v after=%v err=%v", progressToPreserve, preservedProgress, err)
	}
	completedTelephone, err := opuser.FindUserTelephone(ctx, testConfig.TestUserSuid, telephoneToComplete.GetTelephoneId())
	if err != nil || completedTelephone.GetTelephoneStatusType() != penum.TelephoneStatusType_TelephoneStatusType_Finished ||
		completedTelephone.GetScheduledTime() < secondGodStarted || completedTelephone.GetScheduledTime() > secondGodFinished {
		t.Fatalf("god mode retained an impossible telephone schedule: telephone=%v err=%v", completedTelephone, err)
	}
	legacyTicketItems, err := opuser.FindUserItems(ctx, testConfig.TestUserSuid, ticketItemIds[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(legacyTicketItems) != 0 {
		t.Fatalf("legacy god gacha ticket was not removed: %v", legacyTicketItems)
	}
	legitimateTicketItems, err := opuser.FindUserItems(ctx, testConfig.TestUserSuid, ticketItemIds[1])
	if err != nil {
		t.Fatal(err)
	}
	smallPermanentFound := false
	expiringFound := false
	for _, item := range legitimateTicketItems {
		if item.GetExpiredTime() == 0 && item.GetAmount() == 1 {
			smallPermanentFound = true
		}
		if item.GetExpiredTime() == legitimateTicketExpiration && item.GetAmount() == makinouser.GodMinimumResourceAmount {
			expiringFound = true
		}
	}
	if !smallPermanentFound || !expiringFound {
		t.Fatalf("legitimate gacha tickets were not preserved: %v", legitimateTicketItems)
	}

	maxLevel, err := opmaster.FindMaxManagerLevel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userData, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if userData.ManagerLevel != maxLevel.Level || userData.ManagerExp != maxLevel.RequiredExp {
		t.Fatalf("manager progression is not master-derived: user=%v master=%v", userData, maxLevel)
	}
	if userData.GetPhotoLimitExtendedCount() != preservedPhotoLimitExtendedCount {
		t.Fatalf("god mode reduced photo limit extension count: got %d, want %d",
			userData.GetPhotoLimitExtendedCount(), preservedPhotoLimitExtendedCount)
	}
	assertGodResourceAmounts(t, ctx)
	assertGodCharacterActivityProgress(t, ctx)
	assertGodCompanyProgress(t, ctx)
	assertGodPhotoShootingCatalog(t, ctx)

	masterCards, err := opmaster.FindAllCards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userCards, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(userCards) != len(masterCards) {
		t.Fatalf("card unlock count mismatch: got %d, want %d", len(userCards), len(masterCards))
	}
	masterCostumes, err := opmaster.FindAllCostumes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userCostumes, err := opuser.FindAllUserCostumes(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(userCostumes) != len(masterCostumes) {
		t.Fatalf("costume unlock count mismatch: got %d, want %d", len(userCostumes), len(masterCostumes))
	}
	masterStories, err := opmaster.FindAllStories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userStories, err := opuser.FindAllUserStories(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(userStories) != len(masterStories) {
		t.Fatalf("story unlock count mismatch: got %d, want %d", len(userStories), len(masterStories))
	}
	assertGodMainStoriesRead(t, ctx, userStories)
	assertGodNarrativeAll(t, ctx)

	shopCatalog, err := opsolis.FindAllShopCatalogEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	shopPurchase, err := opuser.FindUserShopPurchase(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	purchasedShopItems := make(map[string]struct{}, len(shopPurchase.PurchasedItemIds))
	for _, id := range shopPurchase.PurchasedItemIds {
		purchasedShopItems[id] = struct{}{}
	}
	userShops, err := opuser.FindUserShopIds(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	userShopSet := make(map[string]struct{}, len(userShops.ShopIds))
	for _, id := range userShops.ShopIds {
		userShopSet[id] = struct{}{}
	}
	userShowcaseShops, err := opuser.FindUserShowcaseToyShopIds(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	userShowcaseShopSet := make(map[string]struct{}, len(userShowcaseShops.ShopIds))
	for _, id := range userShowcaseShops.ShopIds {
		userShowcaseShopSet[id] = struct{}{}
	}
	for _, shop := range shopCatalog {
		for _, item := range shop.Items {
			if _, ok := purchasedShopItems[item.GetId()]; !ok {
				t.Fatalf("shop item %q was not marked purchased by god mode", item.GetId())
			}
		}
		if shop.Info == nil {
			continue
		}
		switch shop.Info.GetType() {
		case penum.ShopType_ShopType_ShowcaseToyShop,
			penum.ShopType_ShopType_DefaultShowcaseToyShop,
			penum.ShopType_ShopType_OnlyShowcaseShop:
			if _, ok := userShowcaseShopSet[shop.ShopId]; !ok {
				t.Fatalf("showcase shop %q was not unlocked by god mode", shop.ShopId)
			}
			if _, duplicated := userShopSet[shop.ShopId]; duplicated {
				t.Fatalf("showcase shop %q was duplicated into normal shops", shop.ShopId)
			}
		default:
			if _, ok := userShopSet[shop.ShopId]; !ok {
				t.Fatalf("shop %q was not unlocked by god mode", shop.ShopId)
			}
		}
	}
	boothCatalog, err := opsolis.FindAllExchangeBooths(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userBooths, err := opuser.FindUserExchangeBooth(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	userBoothSet := make(map[string]struct{}, len(userBooths.BoothIds))
	for _, id := range userBooths.BoothIds {
		userBoothSet[id] = struct{}{}
	}
	for _, booth := range boothCatalog {
		if _, ok := userBoothSet[booth.Id]; !ok {
			t.Fatalf("exchange booth %q was not unlocked by god mode", booth.Id)
		}
	}

	assertGodActivityCatalogs(t, ctx)

	state, err := opuser.FindUserGodState(ctx, testConfig.TestUserSuid)
	if err != nil || !state.UnlockCurrentEvents {
		t.Fatalf("god event marker not enabled: state=%v err=%v", state, err)
	}
	if state.MessageFeature != umodel.GodMessageFeatureAll {
		t.Fatalf("default god message feature = %q, want all", state.MessageFeature)
	}
	if _, err := opuser.FindUserEventInfo(ctx, testConfig.TestUserSuid); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("legacy per-user event snapshot still exists: %v", err)
	}
	homeClient := papi.NewHomeClient(testConfig.GrpcConn)
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	homeResponse, err := homeClient.Enter(grpcCtx, &papi.HomeEnterRequest{})
	if err != nil {
		t.Fatal(err)
	}
	globalEventInfo, err := opsolis.FindHomeEventInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expectedEventInfo := proto.Clone(globalEventInfo).(*papi.HomeEventInfo)
	unlockTestHomeEvents(expectedEventInfo.ProtoReflect())
	if !proto.Equal(homeResponse.EventInfo, expectedEventInfo) {
		t.Fatalf("home events are not based on the current global rotation: got=%v want=%v", homeResponse.EventInfo, expectedEventInfo)
	}
	assertHomeEventsUnlocked(t, homeResponse.EventInfo.ProtoReflect())
	if homeResponse.GetCompanyInfo().GetIsReceivableRewards() ||
		homeResponse.GetCompanyInfo().GetIsReceivableEnjoyRewards() {
		t.Fatalf("god Company rewards were not marked received: %v", homeResponse.GetCompanyInfo())
	}

	areas, err := opmaster.FindAllAreas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var lastMainAreaID string
	var lastMainAreaOrder int32 = -1
	for _, area := range areas {
		if area.Type == penum.AreaType_AreaType_Main && area.Order >= lastMainAreaOrder {
			lastMainAreaID, lastMainAreaOrder = area.Id, area.Order
		}
	}
	currentArea, err := opuser.FindUserCurrentQuestArea(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	lesson, err := opuser.FindUserLessonProgress(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if currentArea.CurrentMainAreaId != lastMainAreaID || lesson.CurrentAreaId != lastMainAreaID {
		t.Fatalf("current main area is not master-derived: current=%q lesson=%q want=%q", currentArea.CurrentMainAreaId, lesson.CurrentAreaId, lastMainAreaID)
	}

	showcaseShops, err := opuser.FindUserShowcaseToyShopIds(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	showcaseSet := make(map[string]struct{}, len(showcaseShops.ShopIds))
	for _, id := range showcaseShops.ShopIds {
		showcaseSet[id] = struct{}{}
	}
	for _, shop := range shopCatalog {
		if shop.Info == nil {
			continue
		}
		switch shop.Info.Type {
		case penum.ShopType_ShopType_ShowcaseToyShop,
			penum.ShopType_ShopType_DefaultShowcaseToyShop,
			penum.ShopType_ShopType_OnlyShowcaseShop:
			if _, ok := showcaseSet[shop.ShopId]; !ok {
				t.Fatalf("showcase shop %q was not unlocked by god mode", shop.ShopId)
			}
		}
	}
}

func assertGodCompanyProgress(t *testing.T, ctx context.Context) {
	t.Helper()
	levels, err := opmaster.FindAllCompanyEnjoyCharacterLevels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var maxCharacterPoint int64
	for _, level := range levels {
		maxCharacterPoint = max(maxCharacterPoint, level.GetRequiredPoint())
	}
	masterCharacters, err := opmaster.FindAllCharacters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	masterByID := make(map[string]*pmaster.Character, len(masterCharacters))
	for _, character := range masterCharacters {
		masterByID[character.GetId()] = character
	}
	characters, err := opuser.FindAllUserCharacters(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	var normalCount, normalPointTotal int64
	for _, character := range characters {
		master := masterByID[character.GetCharacterId()]
		if master != nil && master.GetType() == penum.CharacterType_CharacterType_Normal {
			normalCount++
			normalPointTotal += character.GetCompanyEnjoyPoint()
			if character.GetCompanyEnjoyPoint() < maxCharacterPoint {
				t.Fatalf("god Company character point is below the master maximum: %v", character)
			}
		}
		if len(character.GetCompanyBonusParameterInfos()) != 6 {
			t.Fatalf("god Company character bonus vector is incomplete: %v", character)
		}
	}
	if normalCount == 0 {
		t.Fatal("god Company data contains no normal characters")
	}
	wantInheritedPoint := normalPointTotal / normalCount
	for _, character := range characters {
		master := masterByID[character.GetCharacterId()]
		if master != nil && master.GetType() != penum.CharacterType_CharacterType_Normal &&
			character.GetCompanyEnjoyPoint() != wantInheritedPoint {
			t.Fatalf("special Company character point = %d, want normal-idol average %d: %v",
				character.GetCompanyEnjoyPoint(), wantInheritedPoint, character)
		}
	}

	company, err := opuser.FindUserCompany(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if company.GetEnjoyPoint() != normalPointTotal || !company.GetIsAlreadyTransitioned() {
		t.Fatalf("god Company aggregate is inconsistent with character data: %v", company)
	}
	trustLevels, err := opmaster.FindAllCompanyTrustLevels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, level := range trustLevels {
		if company.GetTrustPoint() < level.GetRequiredPoint() {
			t.Fatalf("god Company trust point %d is below master level %d", company.GetTrustPoint(), level.GetLevelKey())
		}
	}
	grades, err := opmaster.FindAllCompanyGrades(ctx)
	if err != nil {
		t.Fatal(err)
	}
	companyPoint := company.GetEnjoyPoint() + company.GetTrustPoint() + company.GetSalaryPoint()
	for _, grade := range grades {
		if companyPoint < grade.GetRequiredPoint() {
			t.Fatalf("god Company point %d is below master grade %q", companyPoint, grade.GetId())
		}
	}
	state, err := opuser.FindUserCompanyRewardState(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.ReceivedGradeIds) != len(grades) || len(state.ReceivedTrustLevelKeys) != len(trustLevels) {
		t.Fatalf("god Company reward claims are incomplete: %v", state)
	}
}

func TestUserGodFanBondFeatures(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	characters, err := opuser.FindAllUserCharacters(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(characters) == 0 {
		t.Fatal("fresh user has no character to exercise fan and bond policies")
	}
	characterID := characters[0].GetCharacterId()
	seed := func(light, middle, heavy, reliability, audience int32) *ptransaction.UserCharacter {
		t.Helper()
		value, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, characterID)
		if err != nil {
			t.Fatal(err)
		}
		value.LightFanAmount = light
		value.MiddleFanAmount = middle
		value.HeavyFanAmount = heavy
		value.Reliability = reliability
		value.AudienceCandidateAmount = audience
		if err := opuser.UpsertUserCharacter(ctx, testConfig.TestUserSuid, value); err != nil {
			t.Fatal(err)
		}
		return proto.Clone(value).(*ptransaction.UserCharacter)
	}
	assertPreserved := func(policy string, want *ptransaction.UserCharacter) {
		t.Helper()
		got, err := opuser.FindUserCharacter(ctx, testConfig.TestUserSuid, characterID)
		if err != nil {
			t.Fatal(err)
		}
		if got.GetLightFanAmount() != want.GetLightFanAmount() ||
			got.GetMiddleFanAmount() != want.GetMiddleFanAmount() ||
			got.GetHeavyFanAmount() != want.GetHeavyFanAmount() ||
			got.GetReliability() != want.GetReliability() ||
			got.GetAudienceCandidateAmount() != want.GetAudienceCandidateAmount() {
			t.Fatalf("god %s changed fan/bond progress: got=%v want=%v", policy, got, want)
		}
	}
	reliabilityMissionIDs := make(map[string]struct{})
	masterMissions, err := opmaster.FindAllMissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, mission := range masterMissions {
		if mission.GetMissionType() == penum.MissionType_MissionType_ReliabilityAchievement {
			reliabilityMissionIDs[mission.GetId()] = struct{}{}
		}
	}
	assertReliabilityMissions := func(wantCompleted bool) {
		t.Helper()
		missions, err := opuser.FindAllUserMissions(ctx, testConfig.TestUserSuid)
		if err != nil {
			t.Fatal(err)
		}
		completed := 0
		for _, mission := range missions {
			if _, reliability := reliabilityMissionIDs[mission.GetMissionId()]; !reliability {
				continue
			}
			completed++
			if !wantCompleted || mission.GetStatusType() != penum.MissionStatusType_MissionStatusType_Received ||
				mission.GetProgress() != mission.GetCurrentThreshold() {
				t.Fatalf("unexpected god reliability mission state: %v", mission)
			}
		}
		if wantCompleted && completed == 0 {
			t.Fatal("god bond all completed no reliability missions")
		}
	}
	for index, policy := range []string{umodel.GodMessageFeatureNone, umodel.GodMessageFeatureEnjoy} {
		want := seed(int32(101+index), int32(202+index), int32(303+index), int32(404+index), int32(505+index))
		callUserGod(t, &admin.UserGodRequest{
			ServerUserId: testConfig.TestUserSuid,
			Feature: admin.GodFeature{
				Message: umodel.GodMessageFeatureNone,
				Fan:     policy,
				Bond:    policy,
			},
		})
		assertPreserved(policy, want)
		assertReliabilityMissions(false)
	}

	settingID, err := config.GetGameSettingId()
	if err != nil {
		t.Fatal(err)
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		t.Fatal(err)
	}
	higherLight := makinouser.GodMinimumLightFanAmount + 111
	higherHeavy := makinouser.GodMinimumHeavyFanAmount + 333
	higherReliability := makinouser.GodMinimumReliability + 444
	seed(
		higherLight,
		makinouser.GodMinimumMiddleFanAmount-1,
		higherHeavy,
		higherReliability,
		1,
	)
	callUserGod(t, &admin.UserGodRequest{
		ServerUserId: testConfig.TestUserSuid,
		Feature: admin.GodFeature{
			Message: umodel.GodMessageFeatureNone,
			Fan:     umodel.GodMessageFeatureAll,
			Bond:    umodel.GodMessageFeatureAll,
		},
	})
	assertAll := func() *ptransaction.UserCharacter {
		t.Helper()
		characters, err := opuser.FindAllUserCharacters(ctx, testConfig.TestUserSuid)
		if err != nil {
			t.Fatal(err)
		}
		var selected *ptransaction.UserCharacter
		for _, character := range characters {
			if character.GetLightFanAmount() < makinouser.GodMinimumLightFanAmount ||
				character.GetMiddleFanAmount() < makinouser.GodMinimumMiddleFanAmount ||
				character.GetHeavyFanAmount() < makinouser.GodMinimumHeavyFanAmount ||
				character.GetReliability() < makinouser.GodMinimumReliability {
				t.Fatalf("god fan/bond minima were not applied to character: %v", character)
			}
			wantAudience := int64(character.GetLightFanAmount())*int64(setting.GetLiveLightFanAmountWeightPermil())/1000 +
				int64(character.GetMiddleFanAmount())*int64(setting.GetLiveMiddleFanAmountWeightPermil())/1000 +
				int64(character.GetHeavyFanAmount())*int64(setting.GetLiveHeavyFanAmountWeightPermil())/1000
			if int64(character.GetAudienceCandidateAmount()) != wantAudience {
				t.Fatalf("character %q audience = %d, want master-derived %d", character.GetCharacterId(), character.GetAudienceCandidateAmount(), wantAudience)
			}
			if character.GetCharacterId() == characterID {
				selected = character
			}
		}
		if selected == nil {
			t.Fatalf("god character %q was not found", characterID)
		}
		return selected
	}
	explicitAll := assertAll()
	if explicitAll.GetLightFanAmount() != higherLight ||
		explicitAll.GetMiddleFanAmount() != makinouser.GodMinimumMiddleFanAmount ||
		explicitAll.GetHeavyFanAmount() != higherHeavy ||
		explicitAll.GetReliability() != higherReliability {
		t.Fatalf("god fan/bond all did not preserve values above its minima: %v", explicitAll)
	}
	assertReliabilityMissions(true)

	seed(1, 2, 3, 4, 5)
	callUserGod(t, &admin.UserGodRequest{
		ServerUserId: testConfig.TestUserSuid,
		Feature:      admin.GodFeature{Message: umodel.GodMessageFeatureNone},
	})
	defaultAll := assertAll()
	if defaultAll.GetLightFanAmount() != makinouser.GodMinimumLightFanAmount ||
		defaultAll.GetMiddleFanAmount() != makinouser.GodMinimumMiddleFanAmount ||
		defaultAll.GetHeavyFanAmount() != makinouser.GodMinimumHeavyFanAmount ||
		defaultAll.GetReliability() != makinouser.GodMinimumReliability {
		t.Fatalf("absent fan/bond policies did not default to all: %v", defaultAll)
	}
}

func TestUserGodMessageNone(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	beforeMessages, err := opuser.FindAllUserMessages(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	beforeSchedules, err := opuser.FindAllUserMessageSchedules(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	beforeProgresses, err := opuser.FindAllUserMessageProgresses(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	beforeTelephones, err := opuser.FindAllUserTelephones(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}

	callUserGod(t, &admin.UserGodRequest{
		ServerUserId: testConfig.TestUserSuid,
		Feature:      admin.GodFeature{Message: umodel.GodMessageFeatureNone},
	})
	homeClient := papi.NewHomeClient(testConfig.GrpcConn)
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	home, err := homeClient.Enter(grpcCtx, &papi.HomeEnterRequest{})
	cancel()
	if err != nil {
		t.Fatalf("Home.Enter after god message none failed: %v", err)
	}
	if len(home.GetCommonResponse().GetUpdatedData().GetMessages()) != 0 ||
		len(home.GetCommonResponse().GetUpdatedData().GetMessageSchedules()) != 0 ||
		len(home.GetCommonResponse().GetUpdatedData().GetTelephones()) != 0 {
		t.Fatalf("god message none unlocked narrative data: %v", home.GetCommonResponse().GetUpdatedData())
	}
	assertSameUserMessages(t, beforeMessages, mustUserMessages(t, ctx))
	assertSameUserMessageSchedules(t, beforeSchedules, mustUserMessageSchedules(t, ctx))
	afterProgresses, err := opuser.FindAllUserMessageProgresses(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterProgresses) != len(beforeProgresses) {
		t.Fatalf("god message none changed progress count: before=%d after=%d", len(beforeProgresses), len(afterProgresses))
	}
	afterTelephones, err := opuser.FindAllUserTelephones(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterTelephones) != len(beforeTelephones) {
		t.Fatalf("god message none changed telephone count: before=%d after=%d", len(beforeTelephones), len(afterTelephones))
	}
	state, err := opuser.FindUserGodState(ctx, testConfig.TestUserSuid)
	if err != nil || state.MessageFeature != umodel.GodMessageFeatureNone {
		t.Fatalf("god message none policy was not persisted: state=%v err=%v", state, err)
	}
}

func TestUserGodMessageEnjoy(t *testing.T) {
	setupTestUser(t)
	callUserGod(t, &admin.UserGodRequest{
		ServerUserId: testConfig.TestUserSuid,
		Feature:      admin.GodFeature{Message: umodel.GodMessageFeatureEnjoy},
	})
	beforeHome := time.Now().UnixMilli()
	homeClient := papi.NewHomeClient(testConfig.GrpcConn)
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, err = homeClient.Enter(grpcCtx, &papi.HomeEnterRequest{})
	cancel()
	if err != nil {
		t.Fatalf("Home.Enter after god message enjoy failed: %v", err)
	}
	afterHome := time.Now().UnixMilli()

	ctx := context.Background()
	masterMessages, err := opmaster.FindAllMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	messageMasterByID := make(map[string]*pmaster.Message, len(masterMessages))
	expectedMessageCount := 0
	for _, master := range masterMessages {
		messageMasterByID[master.GetId()] = master
		if master.GetUnlockConditionId() == "" {
			expectedMessageCount++
		}
	}
	if tutorial := messageMasterByID["message-skr-tutorial-001"]; tutorial != nil && tutorial.GetUnlockConditionId() != "" {
		expectedMessageCount++
	}
	userMessages := mustUserMessages(t, ctx)
	if len(userMessages) != expectedMessageCount {
		t.Fatalf("god enjoy message count = %d, want %d unconditional messages", len(userMessages), expectedMessageCount)
	}
	schedules := mustUserMessageSchedules(t, ctx)
	scheduleByID := make(map[string]*ptransaction.UserMessageSchedule, len(schedules))
	for _, schedule := range schedules {
		scheduleByID[schedule.GetMessageId()] = schedule
	}
	activeByGroup := make(map[string]int)
	for _, message := range userMessages {
		master := messageMasterByID[message.GetMessageId()]
		schedule := scheduleByID[message.GetMessageId()]
		if message.GetMessageId() == "message-skr-tutorial-001" {
			if master == nil || schedule == nil ||
				message.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Received ||
				message.GetArrivedTime() <= 0 || schedule.GetScheduledTime() != message.GetArrivedTime() ||
				len(message.GetSelectMessageDetailIds()) != 0 {
				t.Fatalf("god enjoy changed the initial tutorial conversation: message=%v schedule=%v", message, schedule)
			}
			activeByGroup[master.GetMessageGroupId()]++
			continue
		}
		if master == nil || master.GetUnlockConditionId() != "" {
			t.Fatalf("god enjoy unlocked conditioned/unknown message: %v master=%v", message, master)
		}
		if message.GetMessageStatusType() == penum.MessageStatusType_MessageStatusType_Finished {
			t.Fatalf("god enjoy marked message read: %v", message)
		}
		if schedule == nil {
			t.Fatalf("god enjoy message has no schedule state: %v", message)
		}
		switch message.GetMessageStatusType() {
		case penum.MessageStatusType_MessageStatusType_Unlocked:
			if schedule.GetScheduledTime() != 0 {
				t.Fatalf("queued god enjoy message has a due time: message=%v schedule=%v", message, schedule)
			}
		case penum.MessageStatusType_MessageStatusType_Scheduled:
			activeByGroup[master.GetMessageGroupId()]++
			if message.GetArrivedTime() != 0 || schedule.GetScheduledTime() < beforeHome ||
				schedule.GetScheduledTime() > afterHome+12*time.Hour.Milliseconds() {
				t.Fatalf("god enjoy message delay is outside 0..12h: message=%v schedule=%v", message, schedule)
			}
		case penum.MessageStatusType_MessageStatusType_Received:
			activeByGroup[master.GetMessageGroupId()]++
			if schedule.GetScheduledTime() != message.GetArrivedTime() || message.GetArrivedTime() < beforeHome ||
				message.GetArrivedTime() > afterHome {
				t.Fatalf("immediate god enjoy message has invalid arrival: message=%v schedule=%v", message, schedule)
			}
		default:
			t.Fatalf("god enjoy message has invalid status: %v", message)
		}
	}
	for groupID, count := range activeByGroup {
		if count > 1 {
			t.Fatalf("god enjoy scheduled %d messages for group %q", count, groupID)
		}
	}

	embeddedTelephoneIDs := make(map[string]struct{})
	for _, master := range masterMessages {
		for _, detail := range master.GetDetails() {
			if detail.GetTelephoneId() != "" {
				embeddedTelephoneIDs[detail.GetTelephoneId()] = struct{}{}
			}
		}
	}
	masterTelephones, err := opmaster.FindAllTelephones(ctx)
	if err != nil {
		t.Fatal(err)
	}
	telephoneMasterByID := make(map[string]*pmaster.Telephone, len(masterTelephones))
	expectedTelephoneCount := 0
	for _, master := range masterTelephones {
		telephoneMasterByID[master.GetId()] = master
		_, embedded := embeddedTelephoneIDs[master.GetId()]
		if master.GetUnlockConditionId() == "" && !embedded {
			expectedTelephoneCount++
		}
	}
	userTelephones, err := opuser.FindAllUserTelephones(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(userTelephones) != expectedTelephoneCount {
		t.Fatalf("god enjoy telephone count = %d, want %d standalone unconditional calls", len(userTelephones), expectedTelephoneCount)
	}
	for _, telephone := range userTelephones {
		master := telephoneMasterByID[telephone.GetTelephoneId()]
		_, embedded := embeddedTelephoneIDs[telephone.GetTelephoneId()]
		if master == nil || master.GetUnlockConditionId() != "" || embedded {
			t.Fatalf("god enjoy unlocked an invalid standalone telephone: telephone=%v master=%v", telephone, master)
		}
		if telephone.GetTelephoneStatusType() == penum.TelephoneStatusType_TelephoneStatusType_Scheduled {
			if telephone.GetScheduledTime() < beforeHome || telephone.GetScheduledTime() > afterHome+18*time.Hour.Milliseconds() {
				t.Fatalf("god enjoy telephone delay is outside 0..18h: %v", telephone)
			}
		} else if telephone.GetTelephoneStatusType() != penum.TelephoneStatusType_TelephoneStatusType_Received ||
			telephone.GetScheduledTime() < beforeHome || telephone.GetScheduledTime() > afterHome {
			t.Fatalf("god enjoy telephone has invalid state: %v", telephone)
		}
	}
	state, err := opuser.FindUserGodState(ctx, testConfig.TestUserSuid)
	if err != nil || state.MessageFeature != umodel.GodMessageFeatureEnjoy {
		t.Fatalf("god message enjoy policy was not persisted: state=%v err=%v", state, err)
	}
}

func TestUserGodRejectsInvalidFeatures(t *testing.T) {
	setupTestUser(t)
	testCases := []struct {
		name    string
		feature admin.GodFeature
	}{
		{name: "message", feature: admin.GodFeature{Message: "invalid"}},
		{name: "fan", feature: admin.GodFeature{Fan: "invalid"}},
		{name: "bond", feature: admin.GodFeature{Bond: "invalid"}},
	}
	for _, testCase := range testCases {
		response, err := testConfig.HttpClient.Post(
			fmt.Sprintf("%s/admin/user/god", testConfig.HttpUrl),
			client.Config{Header: getAdminAuthHeader(), Body: &admin.UserGodRequest{
				ServerUserId: testConfig.TestUserSuid,
				Feature:      testCase.feature,
			}},
		)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode() != 400 {
			t.Fatalf("invalid god %s feature returned status %d: %s", testCase.name, response.StatusCode(), response.Body())
		}
	}
	state, err := opuser.FindUserGodState(context.Background(), testConfig.TestUserSuid)
	if err != nil || state.MessageFeature != "" {
		t.Fatalf("invalid god feature mutated policy: state=%v err=%v", state, err)
	}
}

func assertGodCharacterActivityProgress(t *testing.T, ctx context.Context) {
	t.Helper()
	maxLevel, err := opmaster.FindMaxCharacterActivityLevel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	characters, err := opuser.FindAllUserCharacters(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(characters) == 0 {
		t.Fatal("god mode did not create any playable characters")
	}
	for _, character := range characters {
		if character.GetActivityLevel() != maxLevel.GetLevel() {
			t.Fatalf(
				"character %q activity level = %d, want master max %d",
				character.GetCharacterId(),
				character.GetActivityLevel(),
				maxLevel.GetLevel(),
			)
		}
		if character.GetActivityExp() < int64(maxLevel.GetRequiredExp()) {
			t.Fatalf(
				"character %q activity exp = %d, want at least %d",
				character.GetCharacterId(),
				character.GetActivityExp(),
				maxLevel.GetRequiredExp(),
			)
		}
		if !character.GetInActivity() && character.GetActivityStamina() < maxLevel.GetMaxStamina() {
			t.Fatalf(
				"character %q activity stamina = %d, want at least master max %d",
				character.GetCharacterId(),
				character.GetActivityStamina(),
				maxLevel.GetMaxStamina(),
			)
		}
		if character.GetStaminaUpdatedTime() <= 0 {
			t.Fatalf("character %q has no stamina update timestamp", character.GetCharacterId())
		}
	}
}

func assertGodActivityCatalogs(t *testing.T, ctx context.Context) {
	t.Helper()
	assertGodLessonCatalog(t, ctx)
	assertGodFanEventCatalog(t, ctx)
	assertGodPromotionCatalog(t, ctx)
	assertGodRefreshCatalog(t, ctx)
}

func assertGodLessonCatalog(t *testing.T, ctx context.Context) {
	t.Helper()
	lesson, err := opuser.FindUserLessonProgress(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if lesson.GetCurrentAreaId() != "area-main-125" || lesson.GetStepRewardGold() != 359 ||
		lesson.GetStepRewardManagerExp() != 395 || lesson.GetStepRewardCardEnhanceItem() != 1033 {
		t.Fatalf("god lesson catalog was not fully hydrated: %v", lesson)
	}
	lessonInfo, err := opuser.FindUserLessonInfo(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(lessonInfo.GetCampaignEffects()) != 0 || len(lessonInfo.GetCampaignDropRewardIds()) != 0 {
		t.Fatalf("god lesson catalog retained beginner campaign state: %v", lessonInfo)
	}
}

func assertGodFanEventCatalog(t *testing.T, ctx context.Context) {
	t.Helper()
	type profile struct {
		level                 int32
		activityPerformanceID string
		requiredStamina       int32
		stepStamina           int32
		fixedRewardID         string
	}
	profiles := map[string]profile{
		"act-fan-01": {40, "handshake-stage-6", 1800, 10, "rd-act-fan-01-lv40-view"},
		"act-fan-02": {38, "photo-session-stage-6", 1900, 7, "rd-act-fan-02-lv38-view"},
		"act-fan-03": {35, "sign-stage-6", 1800, 5, "rd-act-fan-03-lv35-view"},
	}
	info, err := opuser.FindUserFanEventInfo(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.GetFanEvents()) != len(profiles) {
		t.Fatalf("god fan event count = %d, want %d", len(info.GetFanEvents()), len(profiles))
	}
	if len(info.GetCampaignEffects()) != 0 {
		t.Fatalf("god fan-event catalog retained campaign effects: %v", info.GetCampaignEffects())
	}
	actualByID := make(map[string]*papi.FanEvent, len(info.GetFanEvents()))
	for _, fanEvent := range info.GetFanEvents() {
		actualByID[fanEvent.GetId()] = fanEvent
	}
	masters, err := opmaster.FindAllActivityFanEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(masters) != len(profiles) {
		t.Fatalf("fan event profile count = %d, master count = %d", len(profiles), len(masters))
	}
	for _, master := range masters {
		configured, ok := profiles[master.GetId()]
		if !ok {
			t.Fatalf("fan event %q has no expected god profile", master.GetId())
		}
		var maxLevel int32
		var subName string
		for _, level := range master.GetLevels() {
			if level.GetLevel() > maxLevel {
				maxLevel = level.GetLevel()
				subName = level.GetName()
			}
		}
		if configured.level != maxLevel {
			t.Fatalf("fan event %q configured level = %d, master max = %d", master.GetId(), configured.level, maxLevel)
		}
		actual := actualByID[master.GetId()]
		if actual == nil {
			t.Fatalf("god mode omitted fan event %q", master.GetId())
		}
		expected := &papi.FanEvent{
			Id:                         master.GetId(),
			Name:                       master.GetName(),
			SubName:                    subName,
			Type:                       master.GetType(),
			UnlockConditionId:          master.GetUnlockConditionId(),
			IsUnlocked:                 true,
			ActivityPerformanceId:      configured.activityPerformanceID,
			Level:                      configured.level,
			Exp:                        actual.GetExp(),
			NextLevelRequiredExp:       0,
			RequiredStamina:            configured.requiredStamina,
			NextLevelUnlockConditionId: "",
			IsNextLevelUnlocked:        false,
			StepConsumptionStamina:     configured.stepStamina,
			FixedRewardId:              configured.fixedRewardID,
			ViewConditionId:            master.GetViewConditionId(),
			CampaignDropRewardIds:      []string{},
		}
		if !proto.Equal(actual, expected) {
			t.Fatalf("fan event %q does not match the god catalog: got=%v want=%v", master.GetId(), actual, expected)
		}
	}
}

func assertGodPromotionCatalog(t *testing.T, ctx context.Context) {
	t.Helper()
	type profile struct {
		level                     int32
		activityPerformanceID     string
		stepRewardGold            int32
		stepRewardCardEnhanceItem int32
		multiStepRewardID         string
		maxFanAmount              int32
		subGenre                  string
	}
	profiles := map[string]profile{
		"act-pro-01": {65, "popup", 470, 531, "rd-act-pro-01-65", 405000, "popup"},
		"act-pro-02": {60, "cm", 471, 534, "rd-act-pro-02-60", 405000, "cm"},
		"act-pro-03": {60, "public-radio", 471, 534, "rd-act-pro-03-60", 405000, "public-radio"},
	}
	info, err := opuser.FindUserPromotionInfo(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.GetPromotions()) != len(profiles) {
		t.Fatalf("god promotion count = %d, want %d", len(info.GetPromotions()), len(profiles))
	}
	if len(info.GetCampaignEffects()) != 0 {
		t.Fatalf("god promotion catalog retained beginner campaign effects: %v", info.GetCampaignEffects())
	}
	actualByID := make(map[string]*papi.Promotion, len(info.GetPromotions()))
	for _, promotion := range info.GetPromotions() {
		actualByID[promotion.GetId()] = promotion
	}
	masters, err := opmaster.FindAllActivityPromotions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(masters) != len(profiles) {
		t.Fatalf("promotion profile count = %d, master count = %d", len(profiles), len(masters))
	}
	for _, master := range masters {
		configured, ok := profiles[master.GetId()]
		if !ok {
			t.Fatalf("promotion %q has no expected god profile", master.GetId())
		}
		var maxLevel int32
		var subName string
		for _, level := range master.GetLevels() {
			if level.GetLevel() > maxLevel {
				maxLevel = level.GetLevel()
				subName = level.GetName()
			}
		}
		if configured.level != maxLevel {
			t.Fatalf("promotion %q configured level = %d, master max = %d", master.GetId(), configured.level, maxLevel)
		}
		actual := actualByID[master.GetId()]
		if actual == nil {
			t.Fatalf("god mode omitted promotion %q", master.GetId())
		}
		expected := &papi.Promotion{
			Id:                             master.GetId(),
			Name:                           master.GetName(),
			SubName:                        subName,
			UnlockConditionId:              master.GetUnlockConditionId(),
			IsUnlocked:                     true,
			ActivityPerformanceId:          configured.activityPerformanceID,
			Level:                          configured.level,
			Exp:                            actual.GetExp(),
			NextLevelRequiredExp:           0,
			RequiredStamina:                1,
			NextLevelUnlockConditionId:     "",
			IsNextLevelUnlocked:            false,
			StepConsumptionStamina:         1,
			StepRewardGold:                 configured.stepRewardGold,
			StepRewardCardEnhanceItem:      configured.stepRewardCardEnhanceItem,
			MultiStepRewardId:              configured.multiStepRewardID,
			MaxFanAmount:                   configured.maxFanAmount,
			ViewConditionId:                master.GetViewConditionId(),
			SubGenre:                       configured.subGenre,
			MultiStepCampaignDropRewardIds: []string{},
		}
		if !proto.Equal(actual, expected) {
			t.Fatalf("promotion %q does not match the god catalog: got=%v want=%v", master.GetId(), actual, expected)
		}
	}
}

func assertGodRefreshCatalog(t *testing.T, ctx context.Context) {
	t.Helper()
	type levelProfile struct {
		requiredGold          int32
		activityPerformanceID string
		recoveryAmount        int32
		subGenre              string
	}
	type profile struct {
		requiredMinutes int32
		levels          map[int32]levelProfile
	}
	profiles := map[string]profile{
		"act-ref-01": {
			120,
			map[int32]levelProfile{
				1: {60000, "cafe", 1000, "cafe"},
				2: {210000, "shopping", 1800, "shopping"},
				3: {510000, "bedrock_bath", 2500, "bedrock-bath"},
				4: {1280000, "karaoke", 3200, "karaoke"},
			},
		},
		"act-ref-02": {
			480,
			map[int32]levelProfile{
				1: {130000, "dormitory", 2500, "dormitory"},
				2: {360000, "theme_park", 3800, "theme-park"},
				3: {810000, "hot_spring", 5000, "hot-spring"},
				4: {1770000, "camp", 6900, "camp"},
			},
		},
	}
	info, err := opuser.FindUserRefreshInfo(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.GetRefreshes()) != len(profiles) {
		t.Fatalf("god refresh count = %d, want %d", len(info.GetRefreshes()), len(profiles))
	}
	if len(info.GetCampaignEffects()) != 0 {
		t.Fatalf("god refresh catalog retained campaign effects: %v", info.GetCampaignEffects())
	}
	actualByID := make(map[string]*papi.Refresh, len(info.GetRefreshes()))
	for _, refresh := range info.GetRefreshes() {
		actualByID[refresh.GetId()] = refresh
	}
	masters, err := opmaster.FindAllActivityRefreshes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(masters) != len(profiles) {
		t.Fatalf("refresh profile count = %d, master count = %d", len(profiles), len(masters))
	}
	for _, master := range masters {
		configured, ok := profiles[master.GetId()]
		if !ok {
			t.Fatalf("refresh %q has no expected god profile", master.GetId())
		}
		actual := actualByID[master.GetId()]
		if actual == nil {
			t.Fatalf("god mode omitted refresh %q", master.GetId())
		}
		if len(master.GetLevels()) != len(configured.levels) {
			t.Fatalf(
				"refresh %q configured level count = %d, master count = %d",
				master.GetId(),
				len(configured.levels),
				len(master.GetLevels()),
			)
		}
		levels := make([]*papi.RefreshLevel, 0, len(master.GetLevels()))
		var maxLevel int32
		for _, masterLevel := range master.GetLevels() {
			level, ok := configured.levels[masterLevel.GetLevel()]
			if !ok {
				t.Fatalf("refresh %q level %d has no expected god profile", master.GetId(), masterLevel.GetLevel())
			}
			if masterLevel.GetLevel() > maxLevel {
				maxLevel = masterLevel.GetLevel()
			}
			levels = append(levels, &papi.RefreshLevel{
				Level:                 masterLevel.GetLevel(),
				Name:                  masterLevel.GetName(),
				RequiredGold:          level.requiredGold,
				UnlockConditionId:     masterLevel.GetUnlockConditionId(),
				IsUnlocked:            true,
				ActivityPerformanceId: level.activityPerformanceID,
				RecoveryAmount:        level.recoveryAmount,
				ViewConditionId:       masterLevel.GetViewConditionId(),
				SubGenre:              level.subGenre,
			})
		}
		expected := &papi.Refresh{
			Id:                   master.GetId(),
			Name:                 master.GetName(),
			RequiredMinutes:      configured.requiredMinutes,
			UnlockConditionId:    master.GetUnlockConditionId(),
			IsUnlocked:           true,
			Level:                maxLevel,
			Exp:                  actual.GetExp(),
			NextLevelRequiredExp: 0,
			Levels:               levels,
			ViewConditionId:      master.GetViewConditionId(),
		}
		if !proto.Equal(actual, expected) {
			t.Fatalf("refresh %q does not match the god catalog: got=%v want=%v", master.GetId(), actual, expected)
		}
	}
}

func assertGodResourceAmounts(t *testing.T, ctx context.Context) {
	t.Helper()

	masterAccessories, err := opmaster.FindAllAccessories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userAccessories, err := opuser.FindAllUserAccessories(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	accessoryAmounts := make(map[string]int64, len(userAccessories))
	for _, accessory := range userAccessories {
		accessoryAmounts[accessory.GetAccessoryId()] = accessory.GetAmount()
	}
	for _, master := range masterAccessories {
		if amount := accessoryAmounts[master.GetId()]; amount < makinouser.GodMinimumResourceAmount {
			t.Fatalf("god accessory %q amount = %d, want at least %d", master.GetId(), amount, makinouser.GodMinimumResourceAmount)
		}
	}

	masterItems, err := opmaster.FindAllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userItems, err := opuser.FindAllUserItems(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	permanentItemAmounts := make(map[string]int64, len(userItems))
	for _, item := range userItems {
		if item.GetExpiredTime() == 0 {
			permanentItemAmounts[item.GetItemId()] = item.GetAmount()
		}
	}
	for _, master := range masterItems {
		if master.GetType() == penum.ItemType_ItemType_GachaTicket {
			continue
		}
		if amount := permanentItemAmounts[master.GetId()]; amount < makinouser.GodMinimumResourceAmount {
			t.Fatalf("god item %q amount = %d, want at least %d", master.GetId(), amount, makinouser.GodMinimumResourceAmount)
		}
	}

	userRecipes, err := opuser.FindAllUserPhotoRecipes(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, recipe := range userRecipes {
		if !strings.HasPrefix(recipe.GetPhotoRecipeId(), "photo-recipe-5-rare-5-") && !strings.HasPrefix(recipe.GetPhotoRecipeId(), "photo-recipe-5-rare-4-") && recipe.GetAmount() != 1 {
			t.Fatalf("god photo recipe %q not rarity 4 or 5, or amount != 1 (actual amount = %d)", recipe.GetPhotoRecipeId(), recipe.GetAmount())
		}
	}

	for pointType := penum.PointType_PointType_Gold; pointType <= penum.PointType_PointType_ActivatePhotoShootingStoryTicket; pointType++ {
		point, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, pointType)
		if err != nil {
			t.Fatalf("failed to read god point %s: %v", pointType, err)
		}
		if point.GetAmount() < makinouser.GodMinimumPointAmount {
			t.Fatalf("god point %s amount = %d, want at least %d", pointType, point.GetAmount(), makinouser.GodMinimumPointAmount)
		}
	}

	balance, err := opuser.FindUserBalance(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if balance.GetFreeBalance() < makinouser.GodMinimumBalanceAmount || balance.GetPaidBalance() < makinouser.GodMinimumBalanceAmount {
		t.Fatalf("god balance = free:%d paid:%d, want each at least %d", balance.GetFreeBalance(), balance.GetPaidBalance(), makinouser.GodMinimumBalanceAmount)
	}
}

func assertGodPhotoShootingCatalog(t *testing.T, ctx context.Context) {
	t.Helper()

	masterMusics, err := opmaster.FindAllPhotoQuestMusics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userMusics, err := opuser.FindAllUserPhotoMusics(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	musicStates := make(map[string]bool, len(userMusics))
	for _, music := range userMusics {
		musicStates[music.GetMusicId()] = music.GetIsUnlocked()
	}
	if len(musicStates) != len(masterMusics) {
		t.Fatalf("god photo music count = %d, want %d from PhotoQuestMusic master", len(musicStates), len(masterMusics))
	}
	for _, master := range masterMusics {
		if !musicStates[master.GetId()] {
			t.Fatalf("god photo music %q is absent or locked", master.GetId())
		}
	}

	masterStages, err := opmaster.FindAllPhotoQuestStages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userStages, err := opuser.FindAllUserPhotoStages(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	stageStates := make(map[string]bool, len(userStages))
	for _, stage := range userStages {
		stageStates[stage.GetStageId()] = stage.GetIsUnlocked()
	}
	if len(stageStates) != len(masterStages) {
		t.Fatalf("god photo stage count = %d, want %d from PhotoQuestStage master", len(stageStates), len(masterStages))
	}
	for _, master := range masterStages {
		if !stageStates[master.GetId()] {
			t.Fatalf("god photo stage %q is absent or locked", master.GetId())
		}
	}
}

func assertGodGrantedNoGachaTickets(t *testing.T, ctx context.Context) {
	t.Helper()
	masterItems, err := opmaster.FindAllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ticketIds := make(map[string]struct{})
	for _, item := range masterItems {
		if item.GetType() == penum.ItemType_ItemType_GachaTicket {
			ticketIds[item.GetId()] = struct{}{}
		}
	}
	userItems, err := opuser.FindAllUserItems(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range userItems {
		if _, isGachaTicket := ticketIds[item.GetItemId()]; isGachaTicket {
			t.Fatalf("god mode granted gacha ticket %q with amount %d", item.GetItemId(), item.GetAmount())
		}
	}
}

func assertGodMainStoriesRead(
	t *testing.T,
	ctx context.Context,
	userStories []*ptransaction.UserStory,
) {
	t.Helper()
	parts, err := opmaster.FindAllStoryParts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userStoryById := make(map[string]*ptransaction.UserStory, len(userStories))
	for _, story := range userStories {
		userStoryById[story.GetStoryId()] = story
	}
	mainStoryCount := 0
	for _, part := range parts {
		if part.GetType() != penum.StoryPartType_StoryPartType_Main {
			continue
		}
		for _, chapter := range part.GetChapters() {
			for _, episode := range chapter.GetEpisodes() {
				if episode.GetStoryId() == "" {
					continue
				}
				mainStoryCount++
				story := userStoryById[episode.GetStoryId()]
				if story == nil {
					t.Fatalf("main story episode %q was not unlocked", episode.GetStoryId())
				}
				if story.GetStatusType() != penum.StoryStatusType_StoryStatusType_Read || story.GetIsInvalid() {
					t.Fatalf("main story episode %q was not marked read: %v", episode.GetStoryId(), story)
				}
			}
		}
	}
	if mainStoryCount == 0 {
		t.Fatal("master data contains no main story episodes")
	}
}

func assertGodNarrativeAll(t *testing.T, ctx context.Context) {
	t.Helper()
	masterMessages, err := opmaster.FindAllMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userMessages, err := opuser.FindAllUserMessages(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(userMessages) != len(masterMessages) {
		t.Fatalf("god message count = %d, want %d", len(userMessages), len(masterMessages))
	}
	userMessageByID := make(map[string]*ptransaction.UserMessage, len(userMessages))
	for _, message := range userMessages {
		userMessageByID[message.GetMessageId()] = message
	}
	tutorial := userMessageByID["message-skr-tutorial-001"]
	if tutorial == nil || !slices.Equal(tutorial.GetSelectMessageDetailIds(), []string{"2"}) {
		t.Fatalf("god tutorial history selections = %v, want [2]", tutorial)
	}
	progresses, err := opuser.FindAllUserMessageProgresses(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	progressByMessageID := make(map[string]*umodel.UserMessageProgress, len(progresses))
	for _, progress := range progresses {
		progressByMessageID[progress.MessageId] = progress
	}
	for _, master := range masterMessages {
		message := userMessageByID[master.GetId()]
		if message == nil || message.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Finished ||
			message.GetArrivedTime() <= 0 {
			t.Fatalf("god message %q has invalid history: %v", master.GetId(), message)
		}
		if _, err := messagepath.ExistingHistoryPath(master, message.GetSelectMessageDetailIds()); err != nil {
			t.Fatalf("god message %q has no complete selected path: message=%v err=%v", master.GetId(), message, err)
		}
		progress := progressByMessageID[master.GetId()]
		if progress == nil || !isTestMessageTerminal(master, progress.LastMessageDetailId) ||
			progress.LastMessageDetailTime < message.GetArrivedTime() {
			t.Fatalf("god message %q has invalid progress: %v", master.GetId(), progress)
		}
	}
	schedules, err := opuser.FindAllUserMessageSchedules(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(schedules) != 0 {
		t.Fatalf("god all left stale message schedules: %v", schedules)
	}

	masterTelephones, err := opmaster.FindAllTelephones(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userTelephones, err := opuser.FindAllUserTelephones(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(userTelephones) != len(masterTelephones) {
		t.Fatalf("god telephone count = %d, want %d", len(userTelephones), len(masterTelephones))
	}
	for _, telephone := range userTelephones {
		if telephone.GetTelephoneStatusType() != penum.TelephoneStatusType_TelephoneStatusType_Finished ||
			telephone.GetUnlockedTime() <= 0 || telephone.GetScheduledTime() <= 0 {
			t.Fatalf("god telephone is not finished: %v", telephone)
		}
	}

	conditions, err := opmaster.FindAllConditions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conditionByID := make(map[string]*pmaster.Condition, len(conditions))
	for _, condition := range conditions {
		conditionByID[condition.GetId()] = condition
	}
	storyIDs := make(map[string]struct{})
	for _, master := range masterMessages {
		collectTestMessageStoryPrerequisites(master.GetUnlockConditionId(), conditionByID, storyIDs, make(map[string]struct{}))
	}
	for storyID := range storyIDs {
		story, err := opuser.FindUserStory(ctx, testConfig.TestUserSuid, storyID)
		if err != nil || story.GetStatusType() != penum.StoryStatusType_StoryStatusType_Read || story.GetIsInvalid() {
			t.Fatalf("god message story prerequisite %q is not read: story=%v err=%v", storyID, story, err)
		}
	}
}

func isTestMessageTerminal(master *pmaster.Message, detailID string) bool {
	for _, detail := range master.GetDetails() {
		if detail.GetMessageDetailId() == detailID {
			return len(detail.GetNextMessageDetailIds()) == 0
		}
	}
	return false
}

func mustUserMessages(t *testing.T, ctx context.Context) []*ptransaction.UserMessage {
	t.Helper()
	messages, err := opuser.FindAllUserMessages(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	return messages
}

func mustUserMessageSchedules(t *testing.T, ctx context.Context) []*ptransaction.UserMessageSchedule {
	t.Helper()
	schedules, err := opuser.FindAllUserMessageSchedules(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	return schedules
}

func assertSameUserMessages(t *testing.T, before, after []*ptransaction.UserMessage) {
	t.Helper()
	beforeByID := make(map[string]*ptransaction.UserMessage, len(before))
	for _, message := range before {
		beforeByID[message.GetMessageId()] = message
	}
	if len(beforeByID) != len(after) {
		t.Fatalf("user message count changed: before=%d after=%d", len(beforeByID), len(after))
	}
	for _, message := range after {
		if !proto.Equal(beforeByID[message.GetMessageId()], message) {
			t.Fatalf("user message changed: before=%v after=%v", beforeByID[message.GetMessageId()], message)
		}
	}
}

func assertSameUserMessageSchedules(t *testing.T, before, after []*ptransaction.UserMessageSchedule) {
	t.Helper()
	beforeByID := make(map[string]*ptransaction.UserMessageSchedule, len(before))
	for _, schedule := range before {
		beforeByID[schedule.GetMessageId()] = schedule
	}
	if len(beforeByID) != len(after) {
		t.Fatalf("user message schedule count changed: before=%d after=%d", len(beforeByID), len(after))
	}
	for _, schedule := range after {
		if !proto.Equal(beforeByID[schedule.GetMessageId()], schedule) {
			t.Fatalf("user message schedule changed: before=%v after=%v", beforeByID[schedule.GetMessageId()], schedule)
		}
	}
}

func collectTestMessageStoryPrerequisites(
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
		if storyID := setting.GetReadStory().GetStoryId(); storyID != "" {
			storyIDs[storyID] = struct{}{}
		}
		if dependencyID := setting.GetSatisfyCondition().GetConditionId(); dependencyID != "" {
			collectTestMessageStoryPrerequisites(dependencyID, conditions, storyIDs, visited)
		}
	}
}

func unlockTestHomeEvents(message protoreflect.Message) {
	fields := message.Descriptor().Fields()
	for index := 0; index < fields.Len(); index++ {
		field := fields.Get(index)
		if field.Kind() == protoreflect.BoolKind && (field.JSONName() == "isUnlocked" || field.JSONName() == "unlocked") {
			message.Set(field, protoreflect.ValueOfBool(true))
			continue
		}
		if field.IsList() && field.Message() != nil {
			list := message.Get(field).List()
			for itemIndex := 0; itemIndex < list.Len(); itemIndex++ {
				unlockTestHomeEvents(list.Get(itemIndex).Message())
			}
			continue
		}
		if field.Message() != nil && message.Has(field) {
			unlockTestHomeEvents(message.Get(field).Message())
		}
	}
}

func assertHomeEventsUnlocked(t *testing.T, message protoreflect.Message) {
	t.Helper()
	fields := message.Descriptor().Fields()
	for index := 0; index < fields.Len(); index++ {
		field := fields.Get(index)
		if field.Kind() == protoreflect.BoolKind && (field.JSONName() == "isUnlocked" || field.JSONName() == "unlocked") {
			if !message.Get(field).Bool() {
				t.Fatalf("home event field %s was not unlocked", field.FullName())
			}
			continue
		}
		if field.IsList() && field.Message() != nil {
			list := message.Get(field).List()
			for itemIndex := 0; itemIndex < list.Len(); itemIndex++ {
				assertHomeEventsUnlocked(t, list.Get(itemIndex).Message())
			}
			continue
		}
		if field.Message() != nil && message.Has(field) {
			assertHomeEventsUnlocked(t, message.Get(field).Message())
		}
	}
}
