package gacha

import (
	"context"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *GachaServer) ReceiveFreePremiumGachaTicket(
	ctx context.Context,
	req *papi.GachaReceiveFreePremiumGachaTicketRequest,
) (*papi.GachaReceiveFreePremiumGachaTicketResponse, error) {
	if req.GetGachaId() == "" {
		return nil, status.Error(codes.InvalidArgument, "gachaId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.GachaReceiveFreePremiumGachaTicketResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		catalog, err := viber.LoadGachaMasterCatalog(ctx)
		if err != nil {
			return nil, err
		}
		masterGacha := catalog.Gachas[req.GetGachaId()]
		if masterGacha == nil {
			return nil, status.Error(codes.NotFound, "gacha not found")
		}
		if masterGacha.GetGachaType() != penum.GachaType_GachaType_Premium {
			return nil, status.Error(codes.FailedPrecondition, "gacha is not premium")
		}
		now := time.Now().UnixMilli()
		state, err := currentGachaInfo(ctx, suid, catalog, masterGacha, now)
		if err != nil {
			return nil, err
		}
		if !state.GetUnlocked() {
			return nil, status.Error(codes.PermissionDenied, "gacha is locked")
		}
		premium := state.GetPremiumInfo()
		if premium == nil || !premium.GetIsFreeGachaTicketReceivable() {
			return nil, status.Error(codes.FailedPrecondition, "free premium gacha ticket was already received")
		}
		itemID := premium.GetFreeGachaTicketItemId()
		if itemID == "" {
			return nil, status.Error(codes.FailedPrecondition, "premium gacha has no free ticket")
		}
		if _, err := opmaster.FindItem(ctx, itemID); err != nil {
			return nil, err
		}
		expiresAt := gachaTermEnd(ctx, masterGacha.GetViewConditionId())
		items, err := opuser.FindUserItems(ctx, suid, itemID)
		if err != nil {
			return nil, err
		}
		var item *ptransaction.UserItem
		var before int64
		for _, candidate := range items {
			if candidate.GetExpiredTime() == expiresAt {
				item = candidate
			}
			if candidate.GetExpiredTime() == 0 || candidate.GetExpiredTime() > now {
				before += candidate.GetAmount()
			}
		}
		if item == nil {
			item = &ptransaction.UserItem{ItemId: itemID, ExpiredTime: expiresAt}
		}
		if item.GetAmount() > math.MaxInt64-premiumFreeTicketAmount {
			return nil, status.Error(codes.FailedPrecondition, "premium ticket amount overflows")
		}
		item.Amount += premiumFreeTicketAmount
		if err := opuser.UpsertUserItem(ctx, suid, item); err != nil {
			return nil, err
		}
		premium.IsFreeGachaTicketReceivable = false
		if err := opuser.UpsertUserGachaInfo(ctx, suid, state); err != nil {
			return nil, err
		}
		result := &ptransaction.RewardResult{
			ResourceType: penum.ResourceType_ResourceType_Item,
			ResourceId:   itemID,
			BeforeAmount: before,
			AfterAmount:  before + premiumFreeTicketAmount,
			AmountI64:    premiumFreeTicketAmount,
			IsNew:        before == 0,
		}
		response = &papi.GachaReceiveFreePremiumGachaTicketResponse{
			RewardResults:  []*ptransaction.RewardResult{result},
			CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{Items: []*ptransaction.UserItem{item}}),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
