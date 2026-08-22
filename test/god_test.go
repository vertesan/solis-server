package test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	makinouser "github.com/vertesan/solis-server/internal/makino/user"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/http/admin"

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

	second := callUserGod(t, &admin.UserGodRequest{ServerUserId: testConfig.TestUserSuid})
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
	assertGodResourceAmounts(t, ctx)
	assertGodCharacterActivityProgress(t, ctx)

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

	masterRecipes, err := opmaster.FindAllPhotoRecipes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userRecipes, err := opuser.FindAllUserPhotoRecipes(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	recipeAmounts := make(map[string]int64, len(userRecipes))
	for _, recipe := range userRecipes {
		recipeAmounts[recipe.GetPhotoRecipeId()] = recipe.GetAmount()
	}
	for _, master := range masterRecipes {
		if amount := recipeAmounts[master.GetId()]; amount < makinouser.GodMinimumResourceAmount {
			t.Fatalf("god photo recipe %q amount = %d, want at least %d", master.GetId(), amount, makinouser.GodMinimumResourceAmount)
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
