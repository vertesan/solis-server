package hierarchy

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *HierarchyServer) ListRanking(ctx context.Context, _ *papi.Empty) (*papi.HierarchyRankingResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.HierarchyRankingResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		public, err := opuser.FindUserPublic(ctx, suid)
		if err != nil {
			return nil, err
		}
		hierarchy, err := opuser.FindUserHierarchy(ctx, suid)
		if err != nil {
			return nil, err
		}
		point, err := viber.HierarchyTotalPoint(hierarchy)
		if err != nil {
			return nil, err
		}
		grades, err := opmaster.FindAllHierarchyDetailGrades(ctx)
		if err != nil {
			return nil, err
		}
		grade, err := viber.ResolveHierarchyDetailGrade(grades, point, viber.HierarchyPrivateServerRank)
		if err != nil {
			return nil, err
		}

		deckName, cardInfos, err := hierarchyRankingDeck(ctx, suid)
		if err != nil {
			return nil, err
		}
		response = &papi.HierarchyRankingResponse{
			RankUserInfos: []*papi.HierarchyRankUserInfo{{
				UserId:               public.GetPublicUserId(),
				CurrentRank:          viber.HierarchyPrivateServerRank,
				ManagerName:          user.GetName(),
				ManagerLevel:         user.GetManagerLevel(),
				CurrentDetailGradeId: grade.GetId(),
				CurrentPoint:         point,
				EmblemId:             user.GetEmblemId(),
				DeckName:             deckName,
				CardInfos:            cardInfos,
			}},
			CommonResponse: viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func hierarchyRankingDeck(ctx context.Context, suid string) (string, []*papi.LastCardInfo, error) {
	decks, err := opuser.FindAllUserDecks(ctx, suid)
	if err != nil {
		return "", nil, err
	}
	if len(decks) == 0 {
		return "", []*papi.LastCardInfo{}, nil
	}
	deck := decks[0]
	positions, err := opuser.FindUserDeckPositions(ctx, suid, deck.GetNumber())
	if err != nil {
		return "", nil, err
	}
	sort.SliceStable(positions, func(i, j int) bool { return positions[i].GetPosition() < positions[j].GetPosition() })
	cards, err := opuser.FindAllUserCards(ctx, suid)
	if err != nil {
		return "", nil, err
	}
	cardsByID := make(map[string]*ptransaction.UserCard, len(cards))
	for _, card := range cards {
		cardsByID[card.GetCardId()] = card
	}
	infos := make([]*papi.LastCardInfo, 0, len(positions))
	for _, position := range positions {
		card := cardsByID[position.GetCardId()]
		if card == nil {
			continue
		}
		infos = append(infos, &papi.LastCardInfo{
			Position:       position.GetPosition(),
			CardId:         card.GetCardId(),
			Level:          card.GetLevel(),
			Rarity:         card.GetRarity(),
			EvolutionLevel: card.GetEvolutionLevel(),
		})
	}
	return deck.GetName(), infos, nil
}
