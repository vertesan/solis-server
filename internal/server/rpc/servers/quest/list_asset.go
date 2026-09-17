package quest

import (
	"context"
	"slices"

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
		quest, err := opmaster.FindQuest(ctx, req.GetQuestId())
		if err != nil {
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
		cardCharacters := make([]string, len(positions))
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
		}
		assets, err = questDeckCharacterAssets(ctx, positions, cardCharacters, characters, nil)
		if err != nil {
			return nil, err
		}
		music, err := opmaster.FindMusic(ctx, quest.GetMusicId())
		if err != nil {
			return nil, err
		}
		// Partial editing units still prefetch only their occupied scoring slots.
		// Cosmetic extras belong to a complete five-card live unit.
		if len(assets) == 5 && music.GetDisplayCharacterAmount() > 5 {
			extras, err := prepareExtraLiveAppearances(ctx, suid, music, characters, nil, assets)
			if err != nil {
				return nil, err
			}
			for _, extra := range extras {
				assets = append(assets, &papi.LiveCharacterAssetInfo{CharacterId: extra.GetCharacterId(), CostumeId: extra.GetCostumeId(), HairId: extra.GetHairId()})
			}
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.QuestListAssetResponse{CharacterAssets: assets, CommonResponse: viber.NewCommonResponse(nil)}, nil
}
