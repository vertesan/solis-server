package home

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/birthday"
	"github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/makino/salary"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (s *HomeServer) Enter(ctx context.Context, req *papi.HomeEnterRequest) (*papi.HomeEnterResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.HomeEnterResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
		updatedData := &papi.UpdatedData{}
		gachaInfos, err := viber.ReconcileCurrentGachaInfos(ctx, suid, now)
		if err != nil {
			return nil, err
		}
		fanEventInfo, err := opuser.FindUserFanEventInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		promotionInfo, err := opuser.FindUserPromotionInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		refreshInfo, err := opuser.FindUserRefreshInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		if err := reconcileActivityInfos(ctx, suid, now, fanEventInfo, promotionInfo, refreshInfo); err != nil {
			return nil, err
		}
		if err := reconcileActiveFanEventForHome(ctx, suid, now, fanEventInfo); err != nil {
			return nil, err
		}
		fanEventInfoForResponse := activityFanEventInfoForResponse(fanEventInfo)
		promotionInfoForResponse := activityPromotionInfoForResponse(promotionInfo)
		refreshInfoForResponse, err := activityRefreshInfoForResponse(refreshInfo)
		if err != nil {
			return nil, err
		}
		lessonProgress, err := viber.ReconcileActivityLessonProgress(ctx, suid, now)
		if err != nil {
			return nil, err
		}
		lessonInfo, err := viber.ReconcileActivityLessonInfo(ctx, suid, now)
		if err != nil {
			return nil, err
		}
		lessonInfo.LessonProgress = lessonProgress
		shopIds, err := opuser.FindUserShopIds(ctx, suid)
		if err != nil {
			return nil, err
		}
		exchangeBooth, err := opuser.FindUserExchangeBooth(ctx, suid)
		if err != nil {
			return nil, err
		}
		notiShowcaseShopItem, err := opuser.FindUserNotiShowcaseShopItem(ctx, suid)
		if err != nil {
			return nil, err
		}
		// The private server's Showcase catalog is generated from current master
		// data. Never expose stale, captured item IDs as "new item" badges.
		notiShowcaseShopItem.ShopId = viber.ShowcaseGoodsShopID
		notiShowcaseShopItem.ShopItemIds = []string{}
		eventInfo, err := opsolis.FindHomeEventInfo(ctx)
		if err != nil {
			return nil, err
		}
		eventInfo.LoveEventInfos, err = homeLoveEvents(ctx, suid, now, updatedData)
		if err != nil {
			return nil, err
		}
		profile, err := opuser.FindUserProfile(ctx, suid)
		if err != nil {
			return nil, err
		}
		birthdayEvent, err := birthday.Reconcile(ctx, suid, profile, time.UnixMilli(now))
		if err != nil {
			return nil, err
		}
		eventInfo.HomeBirthdayInfo = birthdayEvent.HomeInfo()
		godState, err := opuser.FindUserGodState(ctx, suid)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		hierarchyNotiInfo, hierarchy, hierarchyChanged, err := viber.ReconcileHierarchyNotification(
			ctx,
			suid,
			viber.HierarchyPrivateServerRank,
		)
		if err != nil {
			return nil, err
		}
		if hierarchyChanged {
			updatedData.Hierarchy = hierarchy
		}
		eventStoryInfos, err := opuser.FindAllUserEventStoryInfos(ctx, suid)
		if err != nil {
			return nil, err
		}
		photoActivities, err := opuser.FindAllUserPhotoActivities(ctx, suid)
		if err != nil {
			return nil, err
		}
		photoMusics, err := opuser.FindAllUserPhotoMusics(ctx, suid)
		if err != nil {
			return nil, err
		}
		photoStages, err := opuser.FindAllUserPhotoStages(ctx, suid)
		if err != nil {
			return nil, err
		}
		extraStoryPartInfos, err := opuser.FindAllUserExtraStoryPartInfos(ctx, suid)
		if err != nil {
			return nil, err
		}
		photoExpressions, err := opuser.FindAllUserPhotoExpressions(ctx, suid)
		if err != nil {
			return nil, err
		}
		masterPhotoActivities, err := opmaster.FindAllPhotoActivities(ctx)
		if err != nil {
			return nil, err
		}
		masterPhotoMusics, err := opmaster.FindAllPhotoQuestMusics(ctx)
		if err != nil {
			return nil, err
		}
		masterPhotoStages, err := opmaster.FindAllPhotoQuestStages(ctx)
		if err != nil {
			return nil, err
		}
		masterPhotoExpressions, err := opmaster.FindAllPhotoExpressions(ctx)
		if err != nil {
			return nil, err
		}
		photoActivities = viber.FilterUserPhotoActivities(photoActivities, masterPhotoActivities)
		photoMusics = viber.FilterUserPhotoMusics(photoMusics, masterPhotoMusics)
		photoStages = viber.FilterUserPhotoStages(photoStages, masterPhotoStages)
		photoExpressions = viber.FilterUserPhotoExpressions(photoExpressions, masterPhotoExpressions)
		showcaseToyShopIds, err := opuser.FindUserShowcaseToyShopIds(ctx, suid)
		if err != nil {
			return nil, err
		}
		unlockedShowcaseNumber, err := opuser.FindUserUnlockedShowcaseNumber(ctx, suid)
		if err != nil {
			return nil, err
		}
		menuBanners, err := opsolis.FindAllMenuBanners(ctx)
		if err != nil {
			return nil, err
		}
		userCurrentQuestArea, err := opuser.FindUserCurrentQuestArea(ctx, suid)
		if err != nil {
			return nil, err
		}
		visibleShopIds, visibleShowcaseShopIds, visibleExchangeBoothIds,
			allShopIds, allShowcaseShopIds, allExchangeBoothIds, err := filterHomeCommerceIDs(
			ctx,
			suid,
			shopIds.ShopIds,
			showcaseToyShopIds.ShopIds,
			exchangeBooth.BoothIds,
		)
		if err != nil {
			return nil, err
		}
		if !slices.Equal(shopIds.ShopIds, allShopIds) {
			if err := opuser.UpsertUserShopId(ctx, &umodel.UserShopId{SUID: suid, ShopIds: allShopIds}); err != nil {
				return nil, err
			}
		}
		if !slices.Equal(showcaseToyShopIds.ShopIds, allShowcaseShopIds) {
			if err := opuser.UpsertUserShowcaseToyShopId(ctx, &umodel.UserShowcaseToyShopId{
				SUID: suid, ShopIds: allShowcaseShopIds,
			}); err != nil {
				return nil, err
			}
		}
		if !slices.Equal(exchangeBooth.BoothIds, allExchangeBoothIds) {
			if err := opuser.UpsertUserExchangeBooth(ctx, &umodel.UserExchangeBooth{
				SUID: suid, BoothIds: allExchangeBoothIds,
			}); err != nil {
				return nil, err
			}
		}
		if err := reconcileNarrativeSchedules(ctx, suid, now, updatedData); err != nil {
			return nil, err
		}
		companyInfo, err := buildCompanyHomeInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		if err := salary.RecordAttendance(ctx, suid, time.UnixMilli(now)); err != nil {
			return nil, err
		}
		payslip, err := salary.PendingPayslip(ctx, suid, time.UnixMilli(now))
		if err != nil {
			return nil, err
		}
		eventInfo.EventMissionInfos, err = viber.ReconcileUserMissions(ctx, suid, now, updatedData)
		if err != nil {
			return nil, err
		}
		dreamMonthly, err := dream.MonthlyCatalog(ctx, now)
		if err != nil {
			return nil, err
		}
		dreamUnlocked, err := viber.NewCommerceConditionEvaluator(ctx, suid, now).Satisfied(dream.UnlockConditionID)
		if err != nil {
			return nil, err
		}
		eventInfo.DreamInfo = &papi.HomeDreamInfo{
			Name: "イメージトレーニング", StartTime: dreamMonthly.StartTime,
			AssetId: "event-dream-defalut", BannerAssetId: dreamMonthly.Group.GetAssetId(), Order: 60,
			UnlockConditionId: dream.UnlockConditionID, IsUnlocked: dreamUnlocked,
			GenerateAreaItemId: dream.ManualItemID, EventMissionId: dreamMonthly.EventMissionID,
		}
		if godState != nil && godState.UnlockCurrentEvents {
			unlockHomeEventInfo(eventInfo.ProtoReflect())
		}

		response = &papi.HomeEnterResponse{
			GachaInfos:              gachaInfos,
			FanEventInfo:            fanEventInfoForResponse,
			PromotionInfo:           promotionInfoForResponse,
			RefreshInfo:             refreshInfoForResponse,
			LessonProgress:          lessonProgress,
			BannerInfo:              &papi.BannerInfo{MenuBanners: menuBanners},
			ShopIds:                 visibleShopIds,
			ExchangeBoothIds:        visibleExchangeBoothIds,
			CurrentMainAreaId:       userCurrentQuestArea.CurrentMainAreaId,
			EventInfo:               eventInfo,
			HierarchyNotiInfo:       hierarchyNotiInfo,
			CurrentTowerQuestId:     userCurrentQuestArea.CurrentTowerQuestId,
			EventStoryInfos:         eventStoryInfos,
			PhotoActivities:         photoActivities,
			PhotoMusics:             photoMusics,
			PhotoStages:             photoStages,
			LessonInfo:              lessonInfo,
			EventButtonDisplayInfos: []*papi.EventButtonDisplayInfo{}, // TODO
			ExtraStoryPartInfos:     extraStoryPartInfos,
			SpecialHomeActionInfos:  []*papi.SpecialHomeActionInfo{}, // TODO
			NotiShowcaseShopItem:    notiShowcaseShopItem,
			ShowcaseToyShopIds:      visibleShowcaseShopIds,
			UnlockedShowcaseNumbers: unlockedShowcaseNumber.Numbers,
			PhotoExpressions:        photoExpressions,
			CompanyInfo:             companyInfo,
			Payslip:                 payslip,
			CommonResponse:          viber.NewCommonResponse(updatedData),
		}
		return response, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// reconcileActivityInfos applies permanent condition-based unlocks and drops
// expired campaign effects before activity catalogs are exposed from Home.
// Unlocks are monotonic: once earned, a later condition change never relocks a
// user's activity.
func reconcileActivityInfos(
	ctx context.Context,
	suid string,
	now int64,
	fanInfo *papi.FanEventInfo,
	promotionInfo *papi.PromotionInfo,
	refreshInfo *papi.RefreshInfo,
) error {
	evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now)
	if err := viber.ReconcileFanEventInfo(ctx, suid, now, evaluator, fanInfo); err != nil {
		return err
	}
	if err := viber.ReconcilePromotionInfo(ctx, suid, now, evaluator, promotionInfo); err != nil {
		return err
	}
	return viber.ReconcileRefreshInfo(ctx, suid, now, evaluator, refreshInfo)
}

// activityFanEventInfoForResponse clones the persisted catalog and hides its
// selectable entries while a fan event is active, matching the client-facing
// shape without discarding the user's stored progression.
func activityFanEventInfoForResponse(info *papi.FanEventInfo) *papi.FanEventInfo {
	if info == nil {
		return nil
	}
	projected := proto.Clone(info).(*papi.FanEventInfo)
	if projected.GetActiveFanEvent() != nil {
		projected.FanEvents = []*papi.FanEvent{}
	}
	return projected
}

// activityPromotionInfoForResponse is the promotion equivalent of
// ActivityFanEventInfoForResponse.
func activityPromotionInfoForResponse(info *papi.PromotionInfo) *papi.PromotionInfo {
	if info == nil {
		return nil
	}
	projected := proto.Clone(info).(*papi.PromotionInfo)
	if projected.GetActivePromotion() != nil {
		projected.Promotions = []*papi.Promotion{}
	}
	return projected
}

// activityRefreshInfoForResponse clones a persisted base-price catalog and
// projects current campaign discounts into the copy returned to the client.
// Persisting only base prices prevents repeated Home calls from compounding a
// discount and keeps StartRefresh charging from a stable source of truth.
func activityRefreshInfoForResponse(info *papi.RefreshInfo) (*papi.RefreshInfo, error) {
	if info == nil {
		return nil, nil
	}
	projected := proto.Clone(info).(*papi.RefreshInfo)
	if projected.GetActiveRefresh() != nil {
		projected.Refreshes = []*papi.Refresh{}
		return projected, nil
	}
	for _, refresh := range projected.GetRefreshes() {
		for _, level := range refresh.GetLevels() {
			cost, err := viber.ActivityRefreshGoldCost(level.GetRequiredGold(), projected.GetCampaignEffects())
			if err != nil {
				return nil, err
			}
			if cost > math.MaxInt32 {
				return nil, status.Error(codes.FailedPrecondition, "discounted refresh price overflows")
			}
			level.RequiredGold = int32(cost)
		}
	}
	return projected, nil
}

func filterHomeCommerceIDs(
	ctx context.Context,
	suid string,
	shopIDs []string,
	_ []string,
	boothIDs []string,
) ([]string, []string, []string, []string, []string, []string, error) {
	now := time.Now().UnixMilli()
	evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now)
	shops, err := opsolis.FindAllShopCatalogEntries(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	legacyShowcaseShopIDs := make(map[string]struct{})
	for _, shop := range shops {
		if shop.Info != nil && viber.IsShowcaseShopType(shop.Info.GetType()) {
			legacyShowcaseShopIDs[shop.ShopId] = struct{}{}
		}
	}
	allShopIDs := make([]string, 0, len(shopIDs)+len(shops))
	shopSet := make(map[string]struct{}, len(shopIDs)+len(shops))
	for _, shopID := range shopIDs {
		_, legacyShowcase := legacyShowcaseShopIDs[shopID]
		if viber.IsShowcaseCommerceShopID(shopID) || legacyShowcase || strings.HasPrefix(shopID, "shop_top_shop-showcase-") {
			continue
		}
		if _, exists := shopSet[shopID]; !exists {
			shopSet[shopID] = struct{}{}
			allShopIDs = append(allShopIDs, shopID)
		}
	}
	for _, shop := range shops {
		if shop.Info == nil {
			continue
		}
		if viber.IsShowcaseShopType(shop.Info.GetType()) {
			continue
		}
		if _, ok := shopSet[shop.ShopId]; !ok {
			shopSet[shop.ShopId] = struct{}{}
			allShopIDs = append(allShopIDs, shop.ShopId)
		}
	}
	visibleShops := make([]string, 0, len(shopIDs))
	for _, shop := range shops {
		if shop.Info == nil {
			continue
		}
		if viber.IsShowcaseShopType(shop.Info.GetType()) {
			continue
		}
		visible, err := evaluator.Satisfied(shop.Info.GetViewConditionId())
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		if !visible {
			continue
		}
		hasContent, err := viber.ShopCatalogHasVisibleContent(evaluator, shop)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		if !hasContent {
			continue
		}
		if _, ok := shopSet[shop.ShopId]; ok {
			visibleShops = append(visibleShops, shop.ShopId)
		}
	}
	showcaseShops, err := viber.FindAvailableShowcaseCommerceShops(ctx, suid)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	allShowcaseShopIDs := make([]string, 0, len(showcaseShops))
	visibleShowcaseShops := make([]string, 0, len(showcaseShops))
	for _, shop := range showcaseShops {
		allShowcaseShopIDs = append(allShowcaseShopIDs, shop.ShopId)
		visible, err := evaluator.Satisfied(shop.Info.GetViewConditionId())
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		if visible {
			visibleShowcaseShops = append(visibleShowcaseShops, shop.ShopId)
		}
	}

	booths, err := opsolis.FindAllExchangeBooths(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	allBoothIDs := append([]string(nil), boothIDs...)
	boothSet := viber.StringSet(allBoothIDs)
	for _, booth := range booths {
		if _, ok := boothSet[booth.GetId()]; !ok {
			boothSet[booth.GetId()] = struct{}{}
			allBoothIDs = append(allBoothIDs, booth.GetId())
		}
	}
	visibleBooths := make([]string, 0, len(boothIDs))
	for _, booth := range booths {
		if _, ok := boothSet[booth.GetId()]; !ok || booth.GetEndTime() > 0 && booth.GetEndTime() <= now {
			continue
		}
		visible, err := evaluator.Satisfied(booth.GetViewConditionId())
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		unlocked, err := evaluator.Satisfied(booth.GetUnlockConditionId())
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		if visible && unlocked {
			visibleBooths = append(visibleBooths, booth.GetId())
		}
	}
	return visibleShops, visibleShowcaseShops, visibleBooths,
		allShopIDs, allShowcaseShopIDs, allBoothIDs, nil
}

// buildCompanyHomeInfo computes the two Company notification badges from
// claimable grade/trust rewards and character-enjoy rewards.
func buildCompanyHomeInfo(ctx context.Context, suid string) (*papi.CompanyInfo, error) {
	unlocked, err := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli()).Satisfied(viber.CompanyUnlockCondition)
	if err != nil {
		return nil, err
	}
	if !unlocked {
		return &papi.CompanyInfo{}, nil
	}
	if err := viber.ReconcileCompanyMissions(ctx, suid); err != nil {
		return nil, err
	}
	info := &papi.CompanyInfo{}
	company, err := opuser.FindUserCompany(ctx, suid)
	if err != nil {
		return nil, err
	}
	state, err := opuser.FindUserCompanyRewardState(ctx, suid)
	if errors.Is(err, mongo.ErrNoDocuments) {
		state = &umodel.UserCompanyRewardState{SUID: suid}
	} else if err != nil {
		return nil, err
	}
	// Only claimable Company tasks affect these badges. Leave unrelated and
	// already-claimed mission documents in MongoDB instead of decoding all of
	// them on every Home entry.
	missions, err := opuser.FindUserMissionsByStatuses(ctx, suid, []penum.MissionStatusType{penum.MissionStatusType_MissionStatusType_NotReceived})
	if err != nil {
		return nil, err
	}
	missionIDs := make([]string, 0, len(missions))
	for _, mission := range missions {
		if mission == nil || mission.GetStatusType() != penum.MissionStatusType_MissionStatusType_NotReceived {
			continue
		}
		missionIDs = append(missionIDs, mission.GetMissionId())
	}
	if len(missionIDs) > 0 {
		masters, err := opmaster.FindMissionsByIds(ctx, missionIDs)
		if err != nil {
			return nil, err
		}
		for _, master := range masters {
			if master.GetCompanyTrustMissionCategoryType() != penum.CompanyTrustMissionCategoryType_CompanyTrustMissionCategoryType_Unknown {
				info.IsReceivableRewards = true
			}
			if master.GetCompanyEnjoyCharacterMissionCategoryType() != penum.CompanyEnjoyCharacterMissionCategoryType_CompanyEnjoyCharacterMissionCategoryType_Unknown {
				info.IsReceivableEnjoyRewards = true
			}
		}
	}

	receivedGrades := stringSet(state.ReceivedGradeIds)
	grades, err := opmaster.FindAllCompanyGrades(ctx)
	if err != nil {
		return nil, err
	}
	total, err := viber.CompanyPointTotal(company)
	if err != nil {
		return nil, err
	}
	for _, grade := range grades {
		if grade.GetAchieveRewardId() != "" && grade.GetRequiredPoint() <= total && !receivedGrades[grade.GetId()] {
			info.IsReceivableRewards = true
			break
		}
	}
	receivedTrust := int32Set(state.ReceivedTrustLevelKeys)
	trustLevels, err := opmaster.FindAllCompanyTrustLevels(ctx)
	if err != nil {
		return nil, err
	}
	for _, level := range trustLevels {
		if level.GetAchieveRewardId() != "" && level.GetRequiredPoint() <= company.GetTrustPoint() && !receivedTrust[level.GetLevelKey()] {
			info.IsReceivableRewards = true
			break
		}
	}
	if info.GetIsReceivableEnjoyRewards() {
		return info, nil
	}

	characters, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		return nil, err
	}
	levels, err := opmaster.FindAllCompanyEnjoyCharacterLevels(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := opmaster.FindAllCompanyEnjoyCharacterLevelSettings(ctx)
	if err != nil {
		return nil, err
	}
	levelByKey := make(map[int32]int64, len(levels))
	for _, level := range levels {
		levelByKey[level.GetLevelKey()] = level.GetRequiredPoint()
	}
	characterPoints := make(map[string]int64, len(characters))
	for _, character := range characters {
		characterPoints[character.GetCharacterId()] = character.GetCompanyEnjoyPoint()
	}
	receivedByCharacter := state.ReceivedEnjoyCharacterLevelKeys
	receivedSets := make(map[string]map[int32]bool, len(receivedByCharacter))
	for characterID, keys := range receivedByCharacter {
		receivedSets[characterID] = int32Set(keys)
	}
	for _, setting := range settings {
		point, owned := characterPoints[setting.GetCharacterId()]
		requiredPoint, levelExists := levelByKey[setting.GetLevelKey()]
		if !owned || !levelExists || setting.GetAchieveRewardId() == "" || point < requiredPoint {
			continue
		}
		if !receivedSets[setting.GetCharacterId()][setting.GetLevelKey()] {
			info.IsReceivableEnjoyRewards = true
			break
		}
	}
	return info, nil
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func int32Set(values []int32) map[int32]bool {
	result := make(map[int32]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

// reconcileActiveFanEventForHome makes Home.Enter observe the same offline
// progress as Activity.LoadFanEvent before its response projection is built.
func reconcileActiveFanEventForHome(ctx context.Context, suid string, now int64, info *papi.FanEventInfo) error {
	if info == nil || info.GetActiveFanEvent() == nil {
		return nil
	}
	progress, err := opuser.FindUserFanEventRun(ctx, suid)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return status.Error(codes.FailedPrecondition, "active fan event run is missing")
		}
		return err
	}
	state, err := viber.FindFanEventRunState(ctx, suid, progress)
	if err != nil {
		return err
	}
	if _, err := viber.ReconcileFanEventProgress(ctx, suid, progress, state, now); err != nil {
		return err
	}
	viber.SyncActiveFanEventInfo(info, progress)
	if err := opuser.UpsertUserFanEventRun(ctx, suid, progress); err != nil {
		return err
	}
	if err := opuser.UpsertUserFanEventRunState(ctx, state); err != nil {
		return err
	}
	return opuser.UpsertUserFanEventInfo(ctx, suid, info)
}

// homeLoveEvents replaces stale captured dates and flags with the supported
// permanent catalog and caller's current progress. Daily missions are refreshed
// before subsequent activity/photo actions can contribute to them.
func homeLoveEvents(ctx context.Context, suid string, now int64, updated *papi.UpdatedData) ([]*papi.HomeLoveEventInfo, error) {
	infos := make([]*papi.HomeLoveEventInfo, 0)
	for _, id := range moshikoi.SupportedLoveIDs() {
		catalog, err := moshikoi.LoadCatalog(ctx, id)
		if err != nil {
			return nil, err
		}
		state, err := moshikoi.LoadState(ctx, suid, id)
		if err != nil {
			return nil, err
		}
		if state.Registered {
			if err := viber.ReconcileLoveMissions(ctx, suid, id, state.Exp, updated); err != nil {
				return nil, err
			}
		}
		info, err := viber.BuildLoveInfo(ctx, suid, catalog, now)
		if err != nil {
			return nil, err
		}
		var extraTitleAssetID string
		if info.GetBaseType() == penum.LoveBaseType_LoveBaseType_Extra {
			extraTitleAssetID = catalog.TitleAssetID
		}
		infos = append(infos, &papi.HomeLoveEventInfo{
			Id: id, Name: catalog.Config.Name, StartTime: info.GetStartTime(), EndTime: 0,
			BannerAssetId: catalog.Config.BannerAssetID, Order: info.GetOrder(),
			UnlockConditionId: info.GetUnlockConditionId(), IsUnlocked: info.GetIsUnlocked(),
			EventMissionIds: info.GetEventMissionIds(), LimitedEventMissionIds: info.GetLimitedEventMissionIds(),
			IsLessonRewardMax: info.GetIsLessonRewardMax(), IsUnplayed: info.GetIsUnplayed(),
			HasHiddenFavorabilityObtainItem: info.GetHasHiddenFavorabilityObtainItem(), BaseType: info.GetBaseType(),
			LimitedExtraTitleAssetId: extraTitleAssetID,
		})
	}
	return infos, nil
}

func unlockHomeEventInfo(message protoreflect.Message) {
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
				unlockHomeEventInfo(list.Get(itemIndex).Message())
			}
			continue
		}
		if field.Message() != nil && message.Has(field) {
			unlockHomeEventInfo(message.Get(field).Message())
		}
	}
}
