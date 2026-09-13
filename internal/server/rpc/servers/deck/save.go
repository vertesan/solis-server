package deck

import (
	"context"
	"math"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	userdeck "github.com/vertesan/solis-server/internal/makino/user/deck"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func (s *DeckServer) Save(ctx context.Context, req *papi.DeckSaveRequest) (*papi.DeckSaveResponse, error) {
	if req.GetNumber() < 0 || len(req.GetUserDeckPositions()) > 5 || !utf8.ValidString(req.GetName()) || strings.ContainsRune(req.GetName(), 0) {
		return nil, viber.InvalidArgumentf("invalid deck number, name, or position count")
	}
	if _, ok := penum.DeckEditType_name[int32(req.GetDeckEditType())]; !ok {
		return nil, viber.InvalidArgumentf("invalid deck edit type")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.DeckSaveResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		decks, err := opuser.FindAllUserDecks(ctx, suid)
		if err != nil {
			return nil, err
		}
		var saved *ptransaction.UserDeck
		var highest int64
		for _, deck := range decks {
			highest = max(highest, int64(deck.GetNumber()))
			if deck.GetNumber() == req.GetNumber() {
				saved = deck
			}
		}
		if req.GetNumber() != 0 && saved == nil {
			return nil, status.Error(codes.NotFound, "deck does not exist")
		}
		if req.GetNumber() == 0 {
			settingID, err := config.GetGameSettingId()
			if err != nil {
				return nil, err
			}
			setting, err := opmaster.FindSetting(ctx, settingID)
			if err != nil {
				return nil, err
			}
			if setting.GetDeckCountMax() <= 0 || len(decks) >= int(setting.GetDeckCountMax()) {
				return nil, status.Error(codes.ResourceExhausted, "saved deck limit reached")
			}
		}
		positions, err := validateDeckPositions(ctx, suid, req.GetUserDeckPositions())
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		if saved == nil {
			user, err := opuser.FindUser(ctx, suid)
			if err != nil {
				return nil, err
			}
			sequence := max(user.GetDeckSequence(), highest)
			if sequence < 0 || sequence >= math.MaxInt32 {
				return nil, viber.FailedPreconditionf("deck sequence is exhausted")
			}
			// Repair a stale sequence without ever reusing a saved deck ID.
			if sequence != user.GetDeckSequence() {
				user.DeckSequence = sequence
				if err := opuser.UpsertUser(ctx, suid, user); err != nil {
					return nil, err
				}
			}
			saved, err = userdeck.SpawnUserDeck(ctx, suid, 0, req.GetName())
			if err != nil {
				return nil, err
			}
			updated.User, err = opuser.FindUser(ctx, suid)
			if err != nil {
				return nil, err
			}
		} else {
			saved.Name = req.GetName()
		}
		previous, err := opuser.FindUserDeckPositions(ctx, suid, saved.GetNumber())
		if err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserDeck(ctx, suid, saved); err != nil {
			return nil, err
		}
		// Requests are complete occupied layouts, including explicit equipment
		// removals. Do not merge equipment back from the previous card/position.
		if err := opuser.DeleteUserDeckPositions(ctx, suid, saved.GetNumber()); err != nil {
			return nil, err
		}
		occupied := make(map[int32]bool, len(positions))
		for _, position := range positions {
			position.Number = saved.GetNumber()
			occupied[position.GetPosition()] = true
		}
		if err := opuser.UpsertManyUserDeckPositions(ctx, suid, positions); err != nil {
			return nil, err
		}
		updated.Decks, updated.DeckPositions = []*ptransaction.UserDeck{saved}, positions
		common := viber.NewCommonResponse(updated)
		for _, position := range previous {
			if !occupied[position.GetPosition()] {
				common.DeletedData.DeckPositions = append(common.DeletedData.DeckPositions, &ptransaction.UserDeckPosition{Number: saved.GetNumber(), Position: position.GetPosition()})
			}
		}
		response = &papi.DeckSaveResponse{CommonResponse: common}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}

func validateDeckPositions(ctx context.Context, suid string, input []*ptransaction.UserDeckPosition) ([]*ptransaction.UserDeckPosition, error) {
	positions := make([]*ptransaction.UserDeckPosition, 0, len(input))
	seenPositions, seenCards := map[int32]bool{}, map[string]bool{}
	for _, position := range input {
		if position == nil || position.GetPosition() < 1 || position.GetPosition() > 5 || seenPositions[position.GetPosition()] || position.GetCardId() == "" || seenCards[position.GetCardId()] {
			return nil, viber.InvalidArgumentf("deck positions and cards must be non-empty and unique, with positions in 1..5")
		}
		seenPositions[position.GetPosition()], seenCards[position.GetCardId()] = true, true
		positions = append(positions, proto.Clone(position).(*ptransaction.UserDeckPosition))
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i].GetPosition() < positions[j].GetPosition() })
	characters, seenPhotos, accessories := map[string]bool{}, map[string]bool{}, map[string]int64{}
	for _, position := range positions {
		card, err := opuser.FindUserCard(ctx, suid, position.GetCardId())
		if err != nil {
			return nil, err
		}
		master, err := opmaster.FindCard(ctx, card.GetCardId())
		if err != nil {
			return nil, err
		}
		character, err := opmaster.FindCharacter(ctx, master.GetCharacterId())
		if err != nil {
			return nil, err
		}
		for index, id := range []string{position.GetPart1AccessoryId(), position.GetPart2AccessoryId()} {
			if id == "" {
				continue
			}
			accessory, err := opmaster.FindAccessory(ctx, id)
			if err != nil {
				return nil, err
			}
			if int32(accessory.GetCategoryType()) != int32(index+1) ||
				accessory.GetCharacterId() != "" && accessory.GetCharacterId() != character.GetId() ||
				accessory.GetCharacterGroupId() != "" && accessory.GetCharacterGroupId() != character.GetCharacterGroupId() {
				return nil, viber.FailedPreconditionf("accessory %q cannot be equipped in this slot", id)
			}
			accessories[id]++
		}
		if len(position.GetPhotoIds()) > int(card.GetPhotoEquipableCount()) {
			return nil, viber.FailedPreconditionf("card %q does not have enough photo slots", card.GetCardId())
		}
		retouched := 0
		for _, id := range position.GetPhotoIds() {
			if id == "" || seenPhotos[id] {
				return nil, viber.InvalidArgumentf("a photo cannot be equipped twice in one unit")
			}
			seenPhotos[id] = true
			photo, err := opuser.FindUserPhoto(ctx, suid, id)
			if err != nil {
				return nil, err
			}
			if len(photo.GetCharacterIds()) > 0 && !slices.Contains(photo.GetCharacterIds(), character.GetId()) {
				return nil, viber.FailedPreconditionf("photo %q cannot be equipped by this idol", id)
			}
			if photo.GetPhotoRecipeId() != "" {
				retouched++
			}
		}
		if retouched > 1 {
			return nil, viber.FailedPreconditionf("only one retouched photo can be equipped per idol")
		}
		if characters[character.GetId()] {
			// The client substitutes the duplicate idol in the live video. Do not
			// apply the original idol's appearance to that substitute.
			position.CostumeId, position.HairId = "", ""
		} else if err := normalizeDeckStyle(ctx, suid, character.GetId(), position); err != nil {
			return nil, err
		}
		characters[character.GetId()] = true
	}
	for id, count := range accessories {
		owned, err := opuser.FindUserAccessory(ctx, suid, id)
		if err != nil {
			return nil, err
		}
		if owned.GetAmount() < count {
			return nil, status.Errorf(codes.ResourceExhausted, "insufficient copies of accessory %q", id)
		}
	}
	return positions, nil
}

func normalizeDeckStyle(ctx context.Context, suid, characterID string, position *ptransaction.UserDeckPosition) error {
	var costume *pmaster.Costume
	if position.GetCostumeId() != "" {
		if _, err := opuser.FindUserCostume(ctx, suid, position.GetCostumeId()); err != nil {
			return err
		}
		var err error
		costume, err = opmaster.FindCostume(ctx, position.GetCostumeId())
		if err != nil {
			return err
		}
		if costume.GetCharacterId() != characterID || strings.HasPrefix(costume.GetCostumeTypeId(), "sd-") {
			return viber.FailedPreconditionf("costume cannot be used by this idol in a live")
		}
		if position.GetHairId() == "" {
			character, err := opuser.FindUserCharacter(ctx, suid, characterID)
			if err != nil {
				return err
			}
			position.HairId = costume.GetDefaultHairId()
			if character.GetLiveHairId() != "" {
				hair, err := opmaster.FindHair(ctx, character.GetLiveHairId())
				if err != nil {
					return err
				}
				if hair.GetCharacterId() == characterID && deckHairFitsCostume(hair, costume.GetId()) {
					position.HairId = hair.GetId()
				}
			}
		}
	}
	if position.GetHairId() != "" {
		if _, err := opuser.FindUserHair(ctx, suid, position.GetHairId()); err != nil {
			return err
		}
		hair, err := opmaster.FindHair(ctx, position.GetHairId())
		if err != nil {
			return err
		}
		if hair.GetCharacterId() != characterID || costume != nil && !deckHairFitsCostume(hair, costume.GetId()) {
			return viber.FailedPreconditionf("hair cannot be worn with this idol/costume")
		}
	}
	return nil
}

func deckHairFitsCostume(hair *pmaster.Hair, costumeID string) bool {
	return !slices.Contains(hair.GetNotWearableCostumeIds(), costumeID) &&
		(len(hair.GetWearableCostumeIds()) == 0 || slices.Contains(hair.GetWearableCostumeIds(), costumeID))
}
