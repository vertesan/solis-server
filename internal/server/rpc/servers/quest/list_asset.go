package quest

import (
	"context"
	"slices"
	"strings"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *QuestServer) ListAsset(ctx context.Context, req *papi.QuestListAssetRequest) (*papi.QuestListAssetResponse, error) {
	if req.GetQuestId() == "" || req.GetDeckNumber() <= 0 {
		return nil, viber.InvalidArgumentf("questId and a saved deck number are required")
	}
	if len(req.GetUserBuddyDeckPositions()) != 0 {
		return nil, viber.FailedPreconditionf("friend buddy decks are unavailable on this server")
	}
	suid := viber.GetSuid(ctx).String()
	var assets []*papi.LiveCharacterAssetInfo
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := opmaster.FindQuest(ctx, req.GetQuestId()); err != nil {
			return nil, err
		}
		if _, err := opuser.FindUserDeck(ctx, suid, req.GetDeckNumber()); err != nil {
			return nil, err
		}
		positions, err := opuser.FindUserDeckPositions(ctx, suid, req.GetDeckNumber())
		if err != nil {
			return nil, err
		}
		slices.SortFunc(positions, func(a, b *ptransaction.UserDeckPosition) int {
			return int(a.GetPosition() - b.GetPosition())
		})
		characters, err := opuser.FindAllUserCharacters(ctx, suid)
		if err != nil {
			return nil, err
		}
		slices.SortFunc(characters, func(a, b *ptransaction.UserCharacter) int {
			return strings.Compare(a.GetCharacterId(), b.GetCharacterId())
		})
		owned := make(map[string]*ptransaction.UserCharacter, len(characters))
		for _, character := range characters {
			owned[character.GetCharacterId()] = character
		}
		cardCharacters := make([]string, len(positions))
		reserved := make(map[string]bool, len(positions))
		for index, position := range positions {
			card, err := opuser.FindUserCard(ctx, suid, position.GetCardId())
			if err != nil {
				return nil, err
			}
			master, err := opmaster.FindCard(ctx, card.GetCardId())
			if err != nil {
				return nil, err
			}
			cardCharacters[index] = master.GetCharacterId()
			reserved[master.GetCharacterId()] = true
		}
		assets = make([]*papi.LiveCharacterAssetInfo, 0, len(positions))
		used := make(map[string]bool, len(positions))
		for index, position := range positions {
			id := cardCharacters[index]
			costumeID, hairID := position.GetCostumeId(), position.GetHairId()
			if used[id] {
				// Official replacement selection is not captured. Choose a stable
				// owned idol, reserving every original idol before filling duplicates.
				id, costumeID, hairID = "", "", ""
				for _, character := range characters {
					if candidate := character.GetCharacterId(); !reserved[candidate] && !used[candidate] {
						id = candidate
						break
					}
				}
			}
			character := owned[id]
			if character == nil {
				return nil, viber.FailedPreconditionf("not enough owned idols for the live appearance")
			}
			asset, err := questCharacterAsset(ctx, character, costumeID, hairID)
			if err != nil {
				return nil, err
			}
			used[id] = true
			assets = append(assets, asset)
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.QuestListAssetResponse{CharacterAssets: assets, CommonResponse: viber.NewCommonResponse(nil)}, nil
}

func questCharacterAsset(ctx context.Context, character *ptransaction.UserCharacter, costumeID, hairID string) (*papi.LiveCharacterAssetInfo, error) {
	master, err := opmaster.FindCharacter(ctx, character.GetCharacterId())
	if err != nil {
		return nil, err
	}
	if costumeID == "" {
		costumeID = character.GetLiveCostumeId()
	}
	if costumeID == "" {
		costumeID = master.GetDefaultLiveCostumeId()
	}
	if costumeID == "" {
		costumeID = master.GetDefaultCostumeId()
	}
	costume, err := opmaster.FindCostume(ctx, costumeID)
	if err != nil {
		return nil, err
	}
	if costume.GetCharacterId() != character.GetCharacterId() || strings.HasPrefix(costume.GetCostumeTypeId(), "sd-") {
		return nil, viber.FailedPreconditionf("deck costume has no usable live model for this idol")
	}
	if hairID == "" {
		hairID = character.GetLiveHairId()
	}
	if hairID == "" {
		hairID = costume.GetDefaultHairId()
	}
	hair, err := opmaster.FindHair(ctx, hairID)
	if err != nil {
		return nil, err
	}
	if hair.GetCharacterId() != character.GetCharacterId() || slices.Contains(hair.GetNotWearableCostumeIds(), costumeID) ||
		len(hair.GetWearableCostumeIds()) > 0 && !slices.Contains(hair.GetWearableCostumeIds(), costumeID) {
		// A global live hairstyle may no longer fit a deck-specific costume.
		hairID = costume.GetDefaultHairId()
	}
	return &papi.LiveCharacterAssetInfo{CharacterId: character.GetCharacterId(), CostumeId: costumeID, HairId: hairID}, nil
}
