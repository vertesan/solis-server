package test

import (
	"context"
	"errors"
	"fmt"
	"testing"

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

	shopCatalog, err := opsolis.FindAllShopCatalogEntries(ctx)
	if err != nil {
		t.Fatal(err)
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

	fanInfo, err := opuser.FindUserFanEventInfo(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	masterFans, err := opmaster.FindAllActivityFanEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	masterFanMax := make(map[string]int32, len(masterFans))
	for _, master := range masterFans {
		for _, level := range master.Levels {
			if level.Level > masterFanMax[master.Id] {
				masterFanMax[master.Id] = level.Level
			}
		}
	}
	for _, fan := range fanInfo.FanEvents {
		if !fan.IsUnlocked {
			t.Fatalf("fan event %q was not unlocked by god mode", fan.Id)
		}
		if fan.Level != masterFanMax[fan.Id] {
			t.Fatalf("fan event %q level = %d, want master max %d", fan.Id, fan.Level, masterFanMax[fan.Id])
		}
	}
	promotionInfo, err := opuser.FindUserPromotionInfo(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	masterPromotions, err := opmaster.FindAllActivityPromotions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	masterPromotionMax := make(map[string]int32, len(masterPromotions))
	for _, master := range masterPromotions {
		for _, level := range master.Levels {
			if level.Level > masterPromotionMax[master.Id] {
				masterPromotionMax[master.Id] = level.Level
			}
		}
	}
	for _, promotion := range promotionInfo.Promotions {
		if !promotion.IsUnlocked {
			t.Fatalf("promotion %q was not unlocked by god mode", promotion.Id)
		}
		if promotion.Level != masterPromotionMax[promotion.Id] {
			t.Fatalf("promotion %q level = %d, want master max %d", promotion.Id, promotion.Level, masterPromotionMax[promotion.Id])
		}
	}
	refreshInfo, err := opuser.FindUserRefreshInfo(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	masterRefreshes, err := opmaster.FindAllActivityRefreshes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	masterRefreshMax := make(map[string]int32, len(masterRefreshes))
	for _, master := range masterRefreshes {
		for _, level := range master.Levels {
			if level.Level > masterRefreshMax[master.Id] {
				masterRefreshMax[master.Id] = level.Level
			}
		}
	}
	for _, refresh := range refreshInfo.Refreshes {
		if !refresh.IsUnlocked {
			t.Fatalf("refresh %q was not unlocked by god mode", refresh.Id)
		}
		if refresh.Level != masterRefreshMax[refresh.Id] {
			t.Fatalf("refresh %q level = %d, want master max %d", refresh.Id, refresh.Level, masterRefreshMax[refresh.Id])
		}
		for _, level := range refresh.Levels {
			if !level.IsUnlocked {
				t.Fatalf("refresh %q level %d was not unlocked by god mode", refresh.Id, level.Level)
			}
		}
	}

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
