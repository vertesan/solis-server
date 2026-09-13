package deck

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *DeckServer) Delete(ctx context.Context, req *papi.DeckDeleteRequest) (*papi.DeckDeleteResponse, error) {
	if req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("a positive deck number is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.DeckDeleteResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		positions, err := opuser.FindUserDeckPositions(ctx, suid, req.GetNumber())
		if err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserDeckPositions(ctx, suid, req.GetNumber()); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUserDeck(ctx, suid, req.GetNumber()); err != nil {
			return nil, err
		}
		common := viber.NewCommonResponse(nil)
		common.DeletedData.Decks = []*ptransaction.UserDeck{{Number: req.GetNumber()}}
		sort.Slice(positions, func(i, j int) bool { return positions[i].GetPosition() < positions[j].GetPosition() })
		for _, position := range positions {
			common.DeletedData.DeckPositions = append(common.DeletedData.DeckPositions, &ptransaction.UserDeckPosition{Number: req.GetNumber(), Position: position.GetPosition()})
		}
		response = &papi.DeckDeleteResponse{CommonResponse: common}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}
