package exercise

import (
	"context"
	"slices"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// ResolvePositions validates the complete editable snapshot, then combines it
// with immutable master positions. Incomplete snapshots are allowed while editing.
func (c *Catalog) ResolvePositions(saved []*papi.ExerciseDeckPosition, requireComplete bool) ([]*papi.ExerciseDeckPosition, error) {
	if len(saved) > 5 {
		return nil, status.Error(codes.InvalidArgument, "exercise deck has more than five positions")
	}
	positions := make(map[int32]*papi.ExerciseDeckPosition, 5)
	usedCards := map[string]bool{}
	for _, fixed := range c.Fixed.GetPositions() {
		if fixed.GetPosition() < 1 || fixed.GetPosition() > 5 || fixed.GetCardId() == "" || positions[fixed.GetPosition()] != nil || usedCards[fixed.GetCardId()] {
			return nil, status.Error(codes.FailedPrecondition, "exercise fixed deck is invalid")
		}
		positions[fixed.GetPosition()] = &papi.ExerciseDeckPosition{Position: fixed.GetPosition(), CardId: fixed.GetCardId()}
		usedCards[fixed.GetCardId()] = true
	}
	photos, accessories := map[string]int32{}, map[string]int32{}
	for _, position := range saved {
		if position == nil || position.GetPosition() < 1 || position.GetPosition() > 5 || positions[position.GetPosition()] != nil {
			return nil, status.Error(codes.InvalidArgument, "exercise position is invalid, duplicated, or fixed")
		}
		if !c.Cards[position.GetCardId()] || usedCards[position.GetCardId()] {
			return nil, status.Error(codes.InvalidArgument, "exercise card is unavailable or duplicated")
		}
		usedCards[position.GetCardId()] = true
		character := c.CardCharacters[position.GetCardId()]
		if len(position.GetPhotoAllInOneIds()) > int(c.PhotoSlots[position.GetCardId()]) {
			return nil, status.Error(codes.InvalidArgument, "exercise card does not have enough photo slots at its rental level")
		}
		for _, id := range position.GetPhotoAllInOneIds() {
			photos[id]++
			if photos[id] > c.Photos[id] {
				return nil, status.Error(codes.InvalidArgument, "exercise photo is unavailable or exceeds its inventory")
			}
			if allowed := c.PhotoMasters[id].GetCharacterIds(); len(allowed) > 0 && !slices.Contains(allowed, character.GetId()) {
				return nil, status.Error(codes.InvalidArgument, "exercise photo cannot be equipped by this idol")
			}
		}
		for slot, id := range []string{position.GetPart1AccessoryId(), position.GetPart2AccessoryId()} {
			if id == "" {
				continue
			}
			accessories[id]++
			category := penum.AccessoryCategoryType_AccessoryCategoryType_Part1
			if slot == 1 {
				category = penum.AccessoryCategoryType_AccessoryCategoryType_Part2
			}
			if accessories[id] > c.Accessories[id] || c.AccessoryMasters[id].GetCategoryType() != category {
				return nil, status.Error(codes.InvalidArgument, "exercise accessory is unavailable, exceeds its inventory, or has the wrong slot")
			}
			accessory := c.AccessoryMasters[id]
			if accessory.GetCharacterId() != "" && accessory.GetCharacterId() != character.GetId() ||
				accessory.GetCharacterGroupId() != "" && accessory.GetCharacterGroupId() != character.GetCharacterGroupId() {
				return nil, status.Error(codes.InvalidArgument, "exercise accessory cannot be equipped by this idol")
			}
		}
		positions[position.GetPosition()] = proto.Clone(position).(*papi.ExerciseDeckPosition)
	}
	if requireComplete && len(positions) != 5 {
		return nil, status.Error(codes.FailedPrecondition, "exercise requires a complete five-card deck")
	}
	result := make([]*papi.ExerciseDeckPosition, 0, len(positions))
	for _, position := range positions {
		result = append(result, position)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].GetPosition() < result[j].GetPosition() })
	return result, nil
}

// CharacterAssets uses the exercise's prescribed clothing, never the player's
// owned wardrobe or current live costume selections.
func CharacterAssets(ctx context.Context, catalog *Catalog, positions []*papi.ExerciseDeckPosition) ([]*papi.LiveCharacterAssetInfo, error) {
	roster, err := opmaster.FindAllExerciseCostumes(ctx)
	if err != nil {
		return nil, err
	}
	appearances := make(map[string]*pmaster.ExerciseCostume, len(roster))
	for _, appearance := range roster {
		appearances[appearance.GetCharacterId()] = appearance
	}
	characters := make([]string, len(positions))
	reserved := make(map[string]bool, len(positions))
	for i, position := range positions {
		if character := catalog.CardCharacters[position.GetCardId()]; character != nil {
			characters[i] = character.GetId()
		} else {
			card, err := opmaster.FindCard(ctx, position.GetCardId())
			if err != nil {
				return nil, err
			}
			characters[i] = card.GetCharacterId()
		}
		reserved[characters[i]] = true
	}
	assets := make([]*papi.LiveCharacterAssetInfo, 0, len(positions))
	used := make(map[string]bool, len(positions))
	for _, id := range characters {
		if used[id] {
			// Reserve every original idol before choosing a substitute so that
			// an early duplicate cannot steal a later scoring card's appearance.
			id = ""
			for _, candidate := range roster {
				if !reserved[candidate.GetCharacterId()] && !used[candidate.GetCharacterId()] {
					id = candidate.GetCharacterId()
					break
				}
			}
		}
		appearance := appearances[id]
		if appearance == nil {
			return nil, status.Error(codes.FailedPrecondition, "exercise has no available character appearance")
		}
		costume, err := opmaster.FindCostume(ctx, appearance.GetCostumeId())
		if err != nil {
			return nil, err
		}
		used[id] = true
		assets = append(assets, &papi.LiveCharacterAssetInfo{CharacterId: id, CostumeId: costume.GetId(), HairId: costume.GetDefaultHairId()})
	}
	return assets, nil
}
