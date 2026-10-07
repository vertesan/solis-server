package exercise

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

type Catalog struct {
	Exercise         *pmaster.Exercise
	Quest            *pmaster.Quest
	User             *pmaster.ExerciseUser
	Fixed            *pmaster.ExerciseDeck
	Cards            map[string]bool
	Photos           map[string]int32
	Accessories      map[string]int32
	AccessoryMasters map[string]*pmaster.Accessory
	PhotoMasters     map[string]*pmaster.PhotoAllInOne
	CardCharacters   map[string]*pmaster.Character
	PhotoSlots       map[string]int32
}

func LoadCatalog(ctx context.Context, id string) (*Catalog, error) {
	row, err := opmaster.FindExercise(ctx, id)
	if err != nil {
		return nil, err
	}
	c := &Catalog{
		Exercise: row, Cards: map[string]bool{}, Photos: map[string]int32{}, Accessories: map[string]int32{},
		AccessoryMasters: map[string]*pmaster.Accessory{}, PhotoMasters: map[string]*pmaster.PhotoAllInOne{},
		CardCharacters: map[string]*pmaster.Character{}, PhotoSlots: map[string]int32{},
	}
	if c.Quest, err = opmaster.FindQuest(ctx, row.GetQuestId()); err != nil {
		return nil, err
	}
	if c.User, err = opmaster.FindExerciseUser(ctx, row.GetExerciseUserId()); err != nil {
		return nil, err
	}
	if row.GetExerciseDeckId() != "" {
		if c.Fixed, err = opmaster.FindExerciseDeck(ctx, row.GetExerciseDeckId()); err != nil {
			return nil, err
		}
	}
	if row.GetExerciseCardGroupId() != "" {
		group, err := opmaster.FindExerciseCardGroup(ctx, row.GetExerciseCardGroupId())
		if err != nil {
			return nil, err
		}
		for _, id := range group.GetCardIds() {
			c.Cards[id] = true
			card, err := opmaster.FindCard(ctx, id)
			if err != nil {
				return nil, err
			}
			if c.CardCharacters[id], err = opmaster.FindCharacter(ctx, card.GetCharacterId()); err != nil {
				return nil, err
			}
			if c.PhotoSlots[id], _, _, err = usercard.CalcUnlocks(ctx, c.User.GetCardLevel(), card); err != nil {
				return nil, err
			}
		}
	}
	if row.GetExercisePhotoGroupId() != "" {
		group, err := opmaster.FindExercisePhotoGroup(ctx, row.GetExercisePhotoGroupId())
		if err != nil {
			return nil, err
		}
		for _, photo := range group.GetPhotos() {
			id := photo.GetPhotoAllInOneId()
			c.Photos[id] += photo.GetAmount()
			if c.PhotoMasters[id], err = opmaster.FindPhotoAllInOne(ctx, id); err != nil {
				return nil, err
			}
		}
	}
	if row.GetExerciseAccessoryGroupId() != "" {
		group, err := opmaster.FindExerciseAccessoryGroup(ctx, row.GetExerciseAccessoryGroupId())
		if err != nil {
			return nil, err
		}
		for _, accessory := range group.GetAccessories() {
			id := accessory.GetAccessoryId()
			c.Accessories[id] += accessory.GetAmount()
			if c.AccessoryMasters[id], err = opmaster.FindAccessory(ctx, id); err != nil {
				return nil, err
			}
		}
	}
	return c, nil
}

// InitializeUser creates empty editing snapshots from the current catalog.
// Older users and subsequently added exercises also work without a snapshot:
// their first SaveDeck lazily creates the same bounded document.
func InitializeUser(ctx context.Context, suid string) error {
	rows, err := opmaster.FindAllExercises(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := opuser.UpsertUserExerciseDeck(ctx, &umodel.UserExerciseDeck{
			SUID: suid, ExerciseID: row.GetId(), Data: &papi.ExerciseInfo{ExerciseId: row.GetId()},
		}); err != nil {
			return err
		}
	}
	return nil
}
