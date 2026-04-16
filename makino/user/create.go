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

		return nil, nil
	}, readconcern.Local(), writeconcern.W1()) // db.WithTransaction
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}
