package user

import (
	"context"
	"errors"
	"fmt"
	"time"
	"vertesan/solis-server/db/crud/opsolis"
	"vertesan/solis-server/db/crud/opuser"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/transaction"
	"vertesan/solis-server/makino/consts"
	"vertesan/solis-server/makino/user/card"
	"vertesan/solis-server/makino/user/character"
	"vertesan/solis-server/makino/user/costume"
	"vertesan/solis-server/makino/user/deck"
	"vertesan/solis-server/makino/user/tutorial"
	"vertesan/solis-server/proto/penum"
	"vertesan/solis-server/proto/ptransaction"
	"vertesan/solis-server/utils/jwto"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

func CreateUser(publicUserId string, suid string, password string) error {
	count, err := opuser.CountUserAuthByPublicOrServerUserId(context.Background(), publicUserId, suid)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("a user with the same publicUserId or serverUserId already exists")
	}

	now := time.Now()
	milliNow := now.UnixMilli()

	err = transaction.WithTransaction(func(ctx context.Context) (interface{}, error) {
		// userAuth
		if err := opuser.InsertUserAuth(ctx, &ustruct.UserAuth{
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
			DeckSequence:                       1,
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
		for _, cardId := range consts.UserInitialCardIds {
			userCard, err := card.SpawnUserCardById(cardId)
			if err != nil {
				return nil, fmt.Errorf("failed to spawn user card, cardId %q: %w", cardId, err)
			}
			if err := opuser.UpsertUserCard(ctx, suid, userCard); err != nil {
				return nil, err
			}
		}

		// UserCharacter
		for _, characterId := range consts.UserInitialCharacterIds {
			userCharacter, err := character.SpawnUserCharacterById(characterId)
			if err != nil {
				return nil, fmt.Errorf("failed to spawn user character: %w", err)
			}
			if err := opuser.UpsertUserCharacter(ctx, suid, userCharacter); err != nil {
				return nil, err
			}
		}

		// UserCostume
		for _, costumeId := range consts.UserInitialCostumeIds {
			costume, err := costume.SpawnUserCostumeById(costumeId)
			if err != nil {
				return nil, fmt.Errorf("failed to spawn user costume: %w", err)
			}
			if err := opuser.UpsertUserCostume(ctx, suid, costume); err != nil {
				return nil, err
			}
		}

		// UserDeck
		deck, err := deck.SpawnUserDeck(suid, 0, "")
		if err != nil {
			return nil, fmt.Errorf("failed to spawn user deck: %w", err)
		}
		opuser.UpsertUserDeck(ctx, suid, deck)

		// UserDeckPosition
		opuser.UpsertManyUserDeckPositions(
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
		)

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
			MessageStatusType:      penum.MessageStatusType_MessageStatusType_Finished,
			ArrivedTime:            milliNow,
			SelectMessageDetailIds: []string{},
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
			{NotificationType: penum.NotificationType_NotificationType_Gift, Valid: true, StartTime: 0},
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
		if err := opuser.UpsertUserTutorial(ctx, suid, &ptransaction.UserTutorial{
			TutorialType: penum.TutorialType_TutorialType_GameStart,
			Step:         150,
		}); err != nil {
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
		for _, hairId := range consts.UserInitialHairIds {
			if err := opuser.UpsertUserHair(ctx, suid, &ptransaction.UserHair{
				HairId:  hairId,
				Checked: true,
			}); err != nil {
				return nil, err
			}
		}

		// UserCompany
		if err := opuser.UpsertUserCompany(ctx, suid, &ptransaction.UserCompany{}); err != nil {
			return nil, err
		}

		// UserShowcaseLike
		if err := opuser.UpsertUserShowcaseLike(ctx, suid, &ptransaction.UserShowcaseLike{
			RewardLikeCount: 30,
			DailyLikeCount:  0,
		}); err != nil {
			return nil, err
		}

		// UserPhotoPose
		for _, poseId := range consts.UserInitialPhotoPoseIds {
			if err := opuser.UpsertUserPhotoPose(ctx, suid, &ptransaction.UserPhotoPose{
				PhotoPoseId: poseId,
			}); err != nil {
				return nil, err
			}
		}

		return nil, nil
	}, readconcern.Local(), writeconcern.W1()) // db.WithTransaction
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}
