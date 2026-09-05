package accessory

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *AccessoryServer) Sell(ctx context.Context, req *papi.AccessorySellRequest) (*papi.AccessorySellResponse, error) {
	if req.GetAccessoryId() == "" || req.GetCount() <= 0 {
		return nil, viber.InvalidArgumentf("accessoryId and a positive count are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.AccessorySellResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		master, err := opmaster.FindAccessory(ctx, req.AccessoryId)
		if err != nil {
			return nil, err
		}
		if master.SalePrice <= 0 {
			return nil, viber.FailedPreconditionf("accessory cannot be sold")
		}
		accessory, err := opuser.FindUserAccessory(ctx, suid, req.AccessoryId)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, status.Error(codes.ResourceExhausted, "accessory is not owned")
		}
		if err != nil {
			return nil, err
		}
		count := int64(req.Count)
		if accessory.Amount < count {
			return nil, status.Error(codes.ResourceExhausted, "insufficient accessories to sell")
		}
		decks, err := opuser.FindAllUserDeckPositions(ctx, suid)
		if err != nil {
			return nil, err
		}
		leagueDecks, err := opuser.FindAllUserLeagueDeckPositions(ctx, suid)
		if err != nil {
			return nil, err
		}
		if accessory.Amount-count < reservedAccessoryAmount(req.AccessoryId, decks, leagueDecks) {
			return nil, viber.FailedPreconditionf("equipped accessories cannot be sold")
		}
		accessory.Amount -= count
		if err := opuser.UpsertUserAccessory(ctx, suid, accessory); err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{Accessories: []*ptransaction.UserAccessory{accessory}}
		if err := viber.GrantGold(ctx, suid, count*int64(master.SalePrice), updated); err != nil {
			return nil, err
		}
		response = &papi.AccessorySellResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
