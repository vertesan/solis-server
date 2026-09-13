package user

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/consts"
	"github.com/vertesan/solis-server/internal/makino/moshikoi"
	"github.com/vertesan/solis-server/internal/makino/salary"
	"github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/makino/user/character"
	"github.com/vertesan/solis-server/internal/makino/user/costume"
	"github.com/vertesan/solis-server/internal/makino/user/deck"
	"github.com/vertesan/solis-server/internal/makino/user/extrastory"
	"github.com/vertesan/solis-server/internal/makino/user/tutorial"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/jwto"

	"github.com/google/uuid"
)

func CreateUser(ctx context.Context, publicUserId string, suid string, password string) error {
	now := time.Now()
	milliNow := now.UnixMilli()

	err := transaction.WithTransaction(ctx, func(ctx context.Context) (interface{}, error) {
		count, err := opuser.CountUserAuthByPublicOrServerUserId(ctx, publicUserId, suid)
		if err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, errors.New("a user with the same publicUserId or serverUserId already exists")
		}

		if !regexp.MustCompile(`^[a-zA-Z0-9]{8,16}$`).Match([]byte(password)) {
			return nil, errors.New("password must be alphanumeric characters within the length of 8 - 16")
		}

		// userAuth
		if err := opuser.InsertUserAuth(ctx, &umodel.UserAuth{
			PublicUserId:  publicUserId,
			ServerUserId:  suid,
			Password:      password,
			FirebaseId:    uuid.NewString(),
			RefreshTokens: []string{jwto.CreateSecureRefreshToken()},
			ValidSince:    now.Unix(),
			CreatedAt:     milliNow,
			LastLoginAt:   milliNow,
			LastRefreshAt: now,
			OnetimeToken:  "",
		}); err != nil {
			return nil, err
		}

		divisions, err := opsolis.FindAllDivisions(ctx)
		if err != nil {
			return nil, err
		}
		if len(divisions) < 1 {
			return nil, errors.New("no available divisions")
		}

		// User
		if err := opuser.UpsertUser(ctx, suid, &ptransaction.User{
			Name:                               "牧野航平",
			ManagerExp:                         50,
			DeckSequence:                       0,
			GuildId:                            "",
			LastGuildCheckTime:                 0,
			DivisionId:                         divisions[0].Id,
			FirstDivisionId:                    divisions[0].Id,
			EmblemId:                           "emb-001",
			DeckMaxOverallValue:                0,
			LastCreatedForumThreadDatetime:     0,
			GameStartTime:                      milliNow,
			LastLoginTime:                      milliNow,
			CardSupportLevel:                   1,
			CardSupportReleaseCount:            0,
			NextPhotoImageId:                   uuid.NewString(),
			DivisionMovedTime:                  0,
			IsReviewed:                         true,
			QuestMainAreaLastClearCharacterIds: []string{},
			HighestSalary:                      0,
			TutorialClearedTime:                milliNow,
			ComebackStartTime:                  0,
			PhotoLimitExtendedCount:            0,
			InviteHostUserId:                   "",
			BuddyCardId:                        "card-ai-03-schl-00",
			NextPhotoImageIds: []string{
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
			},
			LastHomeTalkDrawTime: milliNow,
			ManagerLevel:         2,
			CardSupportMaxNumber: 3,
		}); err != nil {
			return nil, err
		}

		// UserItem
		if err := opuser.UpsertManyUserItems(ctx, suid, []*ptransaction.UserItem{
			{
				ItemId:      "item-card_enhance",
				Amount:      1060,
				ExpiredTime: 0,
			},
			{
				ItemId:      "item-photo_shooting-01",
				Amount:      1,
				ExpiredTime: 0,
			},
		}); err != nil {
			return nil, err
		}

		// UserCard
		for _, cardId := range consts.GetUserInitialCardIds() {
			userCard, err := card.SpawnUserCardById(ctx, cardId)
			if err != nil {
				return nil, fmt.Errorf("failed to spawn user card, cardId %q: %w", cardId, err)
			}
			if err := opuser.UpsertUserCard(ctx, suid, userCard); err != nil {
				return nil, err
			}
		}

		// UserCharacter
		for _, characterId := range consts.GetUserInitialCharacterIds() {
			userCharacter, err := character.SpawnUserCharacterById(ctx, characterId)
			if err != nil {
				return nil, fmt.Errorf("failed to spawn user character: %w", err)
			}
			if err := opuser.UpsertUserCharacter(ctx, suid, userCharacter); err != nil {
				return nil, err
			}
		}

		// UserCostume
		for _, costumeId := range consts.GetUserInitialCostumeIds() {
			costume, err := costume.SpawnUserCostumeById(ctx, costumeId)
			if err != nil {
				return nil, fmt.Errorf("failed to spawn user costume: %w", err)
			}
			if err := opuser.UpsertUserCostume(ctx, suid, costume); err != nil {
				return nil, err
			}
		}

		// UserDeck
		deck, err := deck.SpawnUserDeck(ctx, suid, 0, "")
		if err != nil {
			return nil, fmt.Errorf("failed to spawn user deck: %w", err)
		}
		if err := opuser.UpsertUserDeck(ctx, suid, deck); err != nil {
			return nil, err
		}

		// UserDeckPosition
		if err := opuser.UpsertManyUserDeckPositions(
			ctx,
			suid,
			[]*ptransaction.UserDeckPosition{
				{
					Number:           1,
					Position:         1,
					CardId:           "card-skr-03-schl-00",
					CostumeId:        "cos-skr-schl-00",
					HairId:           "hair-skr-schl-00",
					Part1AccessoryId: "",
					Part2AccessoryId: "",
					PhotoIds:         []string{},
				},
				{
					Number:           1,
					Position:         2,
					CardId:           "card-ngs-03-schl-00",
					CostumeId:        "cos-ngs-schl-00",
					HairId:           "hair-ngs-schl-00",
					Part1AccessoryId: "",
					Part2AccessoryId: "",
					PhotoIds:         []string{},
				},
				{
					Number:           1,
					Position:         3,
					CardId:           "card-ktn-03-schl-00",
					CostumeId:        "cos-ktn-schl-00",
					HairId:           "hair-ktn-schl-00",
					Part1AccessoryId: "",
					Part2AccessoryId: "",
					PhotoIds:         []string{},
				},
				{
					Number:           1,
					Position:         4,
					CardId:           "card-chs-03-schl-00",
					CostumeId:        "cos-chs-schl-00",
					HairId:           "hair-chs-schl-00",
					Part1AccessoryId: "",
					Part2AccessoryId: "",
					PhotoIds:         []string{},
				},
				{
					Number:           1,
					Position:         5,
					CardId:           "card-rei-03-schl-00",
					CostumeId:        "cos-rei-schl-00",
					HairId:           "hair-rei-schl-00",
					Part1AccessoryId: "",
					Part2AccessoryId: "",
					PhotoIds:         []string{},
				},
			},
		); err != nil {
			return nil, err
		}

		// UserPublic
		if err := opuser.UpsertUserPublic(ctx, &ptransaction.UserPublic{
			ServerUserId: suid,
			PublicUserId: publicUserId,
		}); err != nil {
			return nil, err
		}

		// UserPoint
		if err := opuser.UpsertUserPoint(ctx, suid, &ptransaction.UserPoint{
			PointType: penum.PointType_PointType_Gold,
			Amount:    1000,
		}); err != nil {
			return nil, err
		}

		// UserStaff
		for _, pt := range []penum.ParameterType{
			penum.ParameterType_ParameterType_Dance,
			penum.ParameterType_ParameterType_Vocal,
			penum.ParameterType_ParameterType_Visual,
			penum.ParameterType_ParameterType_Stamina,
			penum.ParameterType_ParameterType_Mental,
			penum.ParameterType_ParameterType_Technique,
		} {
			if err := opuser.UpsertUserStaff(ctx, suid, &ptransaction.UserStaff{
				ParameterType:       pt,
				Level:               1,
				LevelLimitBreakRank: 0,
			}); err != nil {
				return nil, err
			}
		}

		// UserMessage
		if err := opuser.UpsertUserMessage(ctx, suid, &ptransaction.UserMessage{
			MessageId:              "message-skr-tutorial-001",
			MessageStatusType:      penum.MessageStatusType_MessageStatusType_Received,
			ArrivedTime:            milliNow,
			SelectMessageDetailIds: []string{},
		}); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserMessageSchedule(ctx, suid, &ptransaction.UserMessageSchedule{
			MessageId:     "message-skr-tutorial-001",
			ScheduledTime: milliNow,
		}); err != nil {
			return nil, err
		}

		// UserProfile
		if err := opuser.UpsertUserProfile(ctx, suid, &ptransaction.UserProfile{
			Message:                 "よろしくお願いします",
			BackgroundType:          penum.ProfileBackgroundType_ProfileBackgroundType_Default,
			LayoutType:              penum.ProfileLayoutType_ProfileLayoutType_A,
			TwitterInfo:             &ptransaction.TwitterInfo{},
			InformationType:         penum.ProfileInformationType_ProfileInformationType_Many1,
			ColorType:               penum.ProfileColorType_ProfileColorType_Black1,
			FavoriteCardId:          "",
			FavoritePhotoId:         "",
			FavoriteCharacterIds:    []string{},
			BirthMonth:              0,
			BirthDay:                0,
			DecorationId:            "",
			FavoriteCardDisplayType: 0,
		}); err != nil {
			return nil, err
		}

		// UserEmblem
		if err := opuser.UpsertUserEmblem(ctx, suid, &ptransaction.UserEmblem{
			EmblemId: "emb-001",
		}); err != nil {
			return nil, err
		}

		// UserBalance
		if err := opuser.UpsertUserBalance(ctx, suid, &ptransaction.UserBalance{
			FreeBalance: 0,
			PaidBalance: 0,
		}); err != nil {
			return nil, err
		}

		// UserTotalCount
		if err := opuser.UpsertUserTotalCount(ctx, suid, &ptransaction.UserTotalCount{
			LoginCount:      1,
			PointGoldAmount: 1000,
		}); err != nil {
			return nil, err
		}

		// UserPhotoReport
		if err := opuser.UpsertUserPhotoReport(ctx, suid, &ptransaction.UserPhotoReport{
			Photos:   []*ptransaction.UserPhoto{},
			Received: false,
		}); err != nil {
			return nil, err
		}

		// UserNotification
		if err := opuser.UpsertManyUserNotifications(ctx, suid, []*ptransaction.UserNotification{
			{NotificationType: penum.NotificationType_NotificationType_Activity, Valid: false, StartTime: 0},
			{NotificationType: penum.NotificationType_NotificationType_LoginBonus, Valid: true, StartTime: 0},
			{NotificationType: penum.NotificationType_NotificationType_Gift, Valid: false, StartTime: 0},
		}); err != nil {
			return nil, err
		}

		// UserHierarchy
		if err := opuser.UpsertUserHierarchy(ctx, suid, &ptransaction.UserHierarchy{
			ReceivedRewardIds:         []string{},
			ReceivedDivisionRewardIds: []string{},
		}); err != nil {
			return nil, err
		}

		// UserTutorial
		userTutorials, err := tutorial.SpawnUserInitialTutorial(ctx)
		if err != nil {
			return nil, err
		}
		if err := opuser.UpsertManyUserTutorials(ctx, suid, userTutorials); err != nil {
			return nil, err
		}

		// UserHomePosition
		if err := opuser.UpsertManyUserHomePositions(ctx, suid, []*ptransaction.UserHomePosition{
			{HomePositionType: penum.HomePositionType_HomePositionType_Home, CharacterId: "char-skr"},
			{HomePositionType: penum.HomePositionType_HomePositionType_Training, CharacterId: "char-ktn"},
			{HomePositionType: penum.HomePositionType_HomePositionType_Activity, CharacterId: "char-rui"},
		}); err != nil {
			return nil, err
		}

		// UserBuddy
		if err := opuser.UpsertUserBuddy(ctx, suid, &ptransaction.UserBuddy{
			DailyRentalAmount: 0,
		}); err != nil {
			return nil, err
		}

		// UserHair
		for _, hairId := range consts.GetUserInitialHairIds() {
			if err := opuser.UpsertUserHair(ctx, suid, &ptransaction.UserHair{
				HairId:  hairId,
				Checked: true,
			}); err != nil {
				return nil, err
			}
		}

		// UserCompany
		if err := opuser.UpsertUserCompany(ctx, suid, &ptransaction.UserCompany{
			IsAlreadyTransitioned: true,
		}); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserCompanyRewardState(ctx, &umodel.UserCompanyRewardState{
			SUID:                            suid,
			ReceivedGradeIds:                []string{},
			ReceivedTrustLevelKeys:          []int32{},
			ReceivedEnjoyCharacterLevelKeys: map[string][]int32{},
		}); err != nil {
			return nil, err
		}

		// UserShowcase and UserShowcaseMyset
		for number := int32(1); number <= 5; number++ {
			if err := opuser.UpsertUserShowcase(ctx, suid, &ptransaction.UserShowcase{
				Number:          number,
				Name:            fmt.Sprintf("ケース%d", number),
				IsOpen:          true,
				ShowcaseFrameId: "frame-001",
			}); err != nil {
				return nil, err
			}
			if err := opuser.UpsertUserShowcaseMyset(ctx, suid, &ptransaction.UserShowcaseMyset{
				Number: number,
				Name:   fmt.Sprintf("マイセット%d", number),
				IsSave: false,
			}); err != nil {
				return nil, err
			}
		}

		// UserShowcaseLike
		if err := opuser.UpsertUserShowcaseLike(ctx, suid, &ptransaction.UserShowcaseLike{
			RewardLikeCount: 30,
			DailyLikeCount:  0,
		}); err != nil {
			return nil, err
		}

		// UserPhotoPose
		for _, poseId := range consts.GetUserInitialPhotoPoseIds() {
			if err := opuser.UpsertUserPhotoPose(ctx, suid, &ptransaction.UserPhotoPose{
				PhotoPoseId: poseId,
			}); err != nil {
				return nil, err
			}
		}

		// UserFanEventInfo
		if err := opuser.UpsertUserFanEventInfo(ctx, suid, consts.GetUserInitialFanEventInfo()); err != nil {
			return nil, err
		}

		// UserPromotionInfo
		if err := opuser.UpsertUserPromotionInfo(ctx, suid, consts.GetUserInitialPromotionInfo()); err != nil {
			return nil, err
		}

		// UserRefreshInfo
		if err := opuser.UpsertUserRefreshInfo(ctx, suid, consts.GetUserInitialRefreshInfo()); err != nil {
			return nil, err
		}

		// UserLessonProgress
		lessonProgress := consts.GetUserInitialLessonProgress()
		lessonProgress.LastReceiveTime = milliNow
		lessonProgress.MaxRewardTime = milliNow + int64(12*time.Hour/time.Millisecond)
		if err := opuser.UpsertUserLessonProgress(ctx, suid, lessonProgress); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserLessonInfo(ctx, suid, consts.GetUserInitialLessonInfo()); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserActivityLessonDailyState(ctx, &umodel.UserActivityLessonDailyState{
			SUID: suid,
			Day:  now.In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02"),
		}); err != nil {
			return nil, err
		}

		// UserShopIds
		shopCatalog, err := opsolis.FindAllShopCatalogEntries(ctx)
		if err != nil {
			return nil, err
		}
		shopIds := make([]string, 0, len(shopCatalog))
		showcaseShopIds := consts.GetUserInitialShowcaseToyShopIds()
		for _, shop := range shopCatalog {
			if shop.Info == nil {
				continue
			}
			switch shop.Info.GetType() {
			case penum.ShopType_ShopType_ShowcaseToyShop,
				penum.ShopType_ShopType_DefaultShowcaseToyShop,
				penum.ShopType_ShopType_OnlyShowcaseShop:
				continue
			default:
				shopIds = append(shopIds, shop.ShopId)
			}
		}
		if err := opuser.UpsertUserShopId(
			ctx,
			&umodel.UserShopId{SUID: suid, ShopIds: shopIds},
		); err != nil {
			return nil, err
		}

		// UserExchangeBooth
		exchangeCatalog, err := opsolis.FindAllExchangeBooths(ctx)
		if err != nil {
			return nil, err
		}
		boothIds := make([]string, 0, len(exchangeCatalog))
		for _, booth := range exchangeCatalog {
			boothIds = append(boothIds, booth.GetId())
		}
		if err := opuser.UpsertUserExchangeBooth(
			ctx,
			&umodel.UserExchangeBooth{SUID: suid, BoothIds: boothIds},
		); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserExchangePurchase(ctx, &umodel.UserExchangePurchase{
			SUID:           suid,
			PurchaseCounts: map[string]int32{},
			ResetTimes:     map[string]int64{},
		}); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserShopPurchase(ctx, &umodel.UserShopPurchase{
			SUID:                     suid,
			PurchaseCounts:           map[string]int32{},
			ResetTimes:               map[string]int64{},
			PurchasedItemIds:         []string{},
			LoginBonusProgress:       map[string]int32{},
			LoginBonusLastGrantTimes: map[string]int64{},
		}); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserGodState(ctx, &umodel.UserGodState{SUID: suid}); err != nil {
			return nil, err
		}

		// UserNotiStoneShopItem
		if err := opuser.UpsertUserNotiStoneShopItem(ctx, suid, &papi.NotiShopItem{
			ShopId:      "shop_top_dia",
			ShopItemIds: consts.GetUserInitialNotiStoneShopItemIds(),
		}); err != nil {
			return nil, err
		}

		// UserNotiNormalShopItem
		if err := opuser.UpsertUserNotiNormalShopItem(ctx, suid, &papi.NotiShopItem{
			ShopId:      "shop_top_shop",
			ShopItemIds: consts.GetUserInitialNotiNormalShopItemIds(),
		}); err != nil {
			return nil, err
		}

		// UserHierarchyNotiInfo
		if err := opuser.UpsertUserHierarchyNotiInfo(ctx, suid, &papi.HierarchyNotiInfo{
			CurrentDetailGradeId: consts.GetUserInitialHierarchyNotiCurrentDetailGradeId(),
		}); err != nil {
			return nil, err
		}

		// UserEventStoryInfo
		eventStories, err := opmaster.FindAllEventStories(ctx)
		if err != nil {
			return nil, err
		}
		eventStoryInfos := make([]*papi.EventStoryInfo, 0, len(eventStories))
		for _, es := range eventStories {
			episodes := make([]*papi.EventStoryEpisodeInfo, 0, len(es.Episodes))
			for _, ep := range es.Episodes {
				episodes = append(episodes, &papi.EventStoryEpisodeInfo{
					Episode:  ep.Episode,
					Unlocked: true,
				})
			}
			eventStoryInfos = append(eventStoryInfos, &papi.EventStoryInfo{
				Id:       es.Id,
				Episodes: episodes,
			})
		}
		if err := opuser.UpsertManyUserEventStoryInfos(ctx, suid, eventStoryInfos); err != nil {
			return nil, err
		}

		// UserPhotoActivity
		photoActivities, err := opmaster.FindAllPhotoActivities(ctx)
		if err != nil {
			return nil, err
		}
		userPhotoActivities := make([]*papi.PhotoActivity, 0, len(photoActivities))
		for _, pa := range photoActivities {
			userPhotoActivities = append(userPhotoActivities, &papi.PhotoActivity{
				PhotoActivityId: pa.Id,
				IsUnlocked:      true,
			})
		}
		if err := opuser.UpsertManyUserPhotoActivities(ctx, suid, userPhotoActivities); err != nil {
			return nil, err
		}

		// UserPhotoMusic
		musics, err := opmaster.FindAllMusics(ctx)
		if err != nil {
			return nil, err
		}
		photoMusics := make([]*papi.PhotoMusic, 0, len(musics))
		for _, m := range musics {
			if !m.Is3DAvailable {
				continue
			}
			photoMusics = append(photoMusics, &papi.PhotoMusic{
				MusicId:    m.Id,
				IsUnlocked: true,
			})
		}
		if err := opuser.UpsertManyUserPhotoMusics(ctx, suid, photoMusics); err != nil {
			return nil, err
		}

		// UserPhotoStage
		if err := opuser.UpsertManyUserPhotoStages(ctx, suid, consts.GetUserInitialPhotoStages()); err != nil {
			return nil, err
		}

		// UserExtraStoryPartInfo
		extraStoryPartInfos, err := extrastory.SpawnUserInitialExtraStoryPartInfos(ctx)
		if err != nil {
			return nil, err
		}
		if err := opuser.UpsertManyUserExtraStoryPartInfos(ctx, suid, extraStoryPartInfos); err != nil {
			return nil, err
		}

		// UserNotiShowcaseShopItem
		if err := opuser.UpsertUserNotiShowcaseShopItem(ctx, suid, consts.GetUserInitialNotiShowcaseShopItem()); err != nil {
			return nil, err
		}

		// UserShowcaseToyShopId
		if err := opuser.UpsertUserShowcaseToyShopId(ctx, &umodel.UserShowcaseToyShopId{
			SUID:    suid,
			ShopIds: showcaseShopIds,
		}); err != nil {
			return nil, err
		}

		// UserUnlockedShowcaseNumber
		if err := opuser.UpsertUserUnlockedShowcaseNumber(ctx, &umodel.UserUnlockedShowcaseNumber{
			SUID:    suid,
			Numbers: consts.GetUserInitialUnlockedShowcaseNumbers(),
		}); err != nil {
			return nil, err
		}

		// UserPhotoExpression
		photoExpressions, err := opmaster.FindAllPhotoExpressions(ctx)
		if err != nil {
			return nil, err
		}
		userPhotoExpressions := make([]*papi.PhotoExpression, 0, len(photoExpressions))
		for _, pe := range photoExpressions {
			userPhotoExpressions = append(userPhotoExpressions, &papi.PhotoExpression{
				PhotoExpressionId: pe.Id,
				IsUnlocked:        true,
			})
		}
		if err := opuser.UpsertManyUserPhotoExpressions(ctx, suid, userPhotoExpressions); err != nil {
			return nil, err
		}

		// UserCurrentQuestArea
		if err := opuser.UpsertUserCurrentQuestArea(ctx, &umodel.UserCurrentQuestArea{
			SUID:                suid,
			CurrentMainAreaId:   consts.GetUserInitialCurrentMainAreaId(),
			CurrentTowerQuestId: consts.GetUserInitialCurrentTowerQuestId(),
		}); err != nil {
			return nil, err
		}

		if err := moshikoi.InitializeUser(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserBirthdayState(ctx, &umodel.UserBirthdayState{SUID: suid}); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserSalary(ctx, salary.NewState(suid, now)); err != nil {
			return nil, err
		}

		// UserDiaryState
		if err := opuser.UpsertUserDiaryState(ctx, &umodel.UserDiaryState{SUID: suid}); err != nil {
			return nil, err
		}

		// UserMarketState and UserExcursionState are mutable per-user runtime state.
		if err := opuser.UpsertUserMarketState(ctx, &umodel.UserMarketState{SUID: suid}); err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserExcursionState(ctx, &umodel.UserExcursionState{
			SUID:            suid,
			OfferedPlaceIds: []string{},
		}); err != nil {
			return nil, err
		}

		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}
