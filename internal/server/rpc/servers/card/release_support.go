package card

import (
	"context"
	"errors"
	"math"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *CardServer) ReleaseSupport(ctx context.Context, req *papi.CardReleaseSupportRequest) (*papi.CardReleaseSupportResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.CardReleaseSupportResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		maximum, err := opmaster.FindMaxCardSupportRelease(ctx)
		if err != nil {
			return nil, err
		}
		if user.GetCardSupportReleaseCount() < 0 || maximum.GetCount() > math.MaxInt32-3 ||
			user.GetCardSupportMaxNumber() != user.GetCardSupportReleaseCount()+3 {
			return nil, viber.FailedPreconditionf("card support slot state is inconsistent")
		}
		if user.GetCardSupportReleaseCount() >= maximum.GetCount() {
			return nil, viber.FailedPreconditionf("all card support slots are unlocked")
		}
		next := user.GetCardSupportReleaseCount() + 1
		// Master count 1 buys slot 4: the first three slots are already open.
		// The linked level-assist spreadsheet agrees with these master costs.
		cost, err := opmaster.FindCardSupportRelease(ctx, next)
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		if req.GetUseStone() {
			if cost.GetStoneAmount() <= 0 {
				return nil, viber.FailedPreconditionf("card support diamond cost is invalid")
			}
			balance, err := opuser.FindUserBalance(ctx, suid)
			if err != nil {
				return nil, err
			}
			if balance.GetFreeBalance() < 0 || balance.GetPaidBalance() < 0 ||
				int64(balance.GetFreeBalance())+int64(balance.GetPaidBalance()) < int64(cost.GetStoneAmount()) {
				return nil, status.Error(codes.ResourceExhausted, "insufficient diamonds for a support slot")
			}
			free := min(balance.GetFreeBalance(), cost.GetStoneAmount())
			balance.FreeBalance -= free
			balance.PaidBalance -= cost.GetStoneAmount() - free
			if err := opuser.UpsertUserBalance(ctx, suid, balance); err != nil {
				return nil, err
			}
			updated.Balance = balance
		} else {
			if cost.GetItemId() == "" || cost.GetItemAmount() <= 0 {
				return nil, viber.FailedPreconditionf("card support point cost is invalid")
			}
			err = viber.ConsumeExchangeResource(ctx, suid, penum.ResourceType_ResourceType_Item, cost.GetItemId(), int64(cost.GetItemAmount()), updated)
		}
		if err != nil {
			if errors.Is(err, viber.ErrExchangeInsufficientResource) {
				return nil, status.Error(codes.ResourceExhausted, "insufficient resources for a support slot")
			}
			return nil, err
		}
		user.CardSupportReleaseCount = next
		user.CardSupportMaxNumber = next + 3
		if err := opuser.UpsertUser(ctx, suid, user); err != nil {
			return nil, err
		}
		updated.User = user
		response = &papi.CardReleaseSupportResponse{CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
