package viber

import (
	"context"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GrantGiftReward applies the mailbox-specific state carried by a UserGift,
// then delegates ordinary resources to the canonical reward grant path.
func GrantGiftReward(
	ctx context.Context,
	suid string,
	gift *ptransaction.UserGift,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	if gift == nil {
		return nil, status.Error(codes.FailedPrecondition, "gift is required")
	}
	if gift.GetAmount() <= 0 || gift.GetAmount() > math.MaxInt32 {
		return nil, status.Error(codes.FailedPrecondition, "gift amount is invalid")
	}
	if gift.GetLimitTime() < 0 {
		return nil, status.Error(codes.FailedPrecondition, "gift limit time is invalid")
	}
	if gift.GetResourceType() == penum.ResourceType_ResourceType_Item && gift.GetLimitTime() > 0 {
		if gift.GetLimitTime() <= time.Now().UnixMilli() {
			return nil, status.Error(codes.FailedPrecondition, "gift has expired")
		}
		result, err := grantExchangeItemWithExpiration(
			ctx,
			suid,
			gift.GetResourceId(),
			gift.GetAmount(),
			gift.GetLimitTime(),
			updated,
		)
		if err != nil {
			return nil, err
		}
		return []*ptransaction.RewardResult{result}, nil
	}
	return GrantGachaReward(ctx, suid, &pmaster.RewardResource{
		ResourceType: gift.GetResourceType(),
		ResourceId:   gift.GetResourceId(),
		Amount:       int32(gift.GetAmount()),
	}, updated)
}
