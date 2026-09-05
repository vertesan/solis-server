package market

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *MarketServer) Purchase(ctx context.Context, req *papi.MarketPurchaseRequest) (*papi.MarketPurchaseResponse, error) {
	if req.GetNumber() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "number must be positive")
	}
	suid := viber.GetSuid(ctx).String()
	now := time.Now()
	catalog, err := loadMarketCatalog()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load market catalog: %v", err)
	}

	var response *papi.MarketPurchaseResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		state, err := findAndRefreshMarketState(ctx, suid, now, catalog)
		if err != nil {
			return nil, err
		}
		item := findMarketItem(state.MarketItems, req.GetNumber())
		if item == nil {
			return nil, status.Error(codes.NotFound, "market item not found")
		}
		if item.GetSoldOut() {
			return nil, errMarketItemSoldOut
		}
		evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now.UnixMilli())
		unlocked := item.GetUnlockConditionId() == catalog.DiscountUnlockConditionID && isJSTMarketDiscountDay(now)
		if !unlocked {
			unlocked, err = evaluator.Satisfied(item.GetUnlockConditionId())
			if err != nil {
				return nil, err
			}
		}
		if !unlocked {
			return nil, status.Error(codes.PermissionDenied, "market item is locked")
		}

		rewards, err := marketRewards(ctx, item.GetRewardId())
		if err != nil {
			return nil, err
		}
		if err := viber.ValidateExchangeResources(item.GetConsumptionResourceType(), rewards); err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		if err := viber.ConsumeExchangeResource(
			ctx,
			suid,
			item.GetConsumptionResourceType(),
			"",
			int64(item.GetDiscountAmount()),
			updated,
		); err != nil {
			return nil, err
		}
		for _, reward := range rewards {
			if _, err := viber.GrantExchangeRewards(ctx, suid, reward, updated); err != nil {
				return nil, err
			}
		}

		totalCount, err := opuser.FindUserTotalCount(ctx, suid)
		if err != nil {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return nil, err
			}
			totalCount = &ptransaction.UserTotalCount{}
		}
		if totalCount.MarketExchangeCount == math.MaxInt64 {
			return nil, status.Error(codes.FailedPrecondition, "market exchange count limit reached")
		}
		totalCount.MarketExchangeCount++
		if err := opuser.UpsertUserTotalCount(ctx, suid, totalCount); err != nil {
			return nil, err
		}
		updated.TotalCount = totalCount
		missions, err := viber.AdvanceUserMissions(
			ctx, suid, penum.MissionType_MissionType_MarketExchangeCount, 1, "",
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = append(updated.Missions, missions...)

		item.SoldOut = true
		if err := opuser.UpsertUserMarketState(ctx, state); err != nil {
			return nil, err
		}
		response = &papi.MarketPurchaseResponse{
			MarketItems:    cloneMarketItems(state.MarketItems),
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		switch {
		case errors.Is(err, errMarketItemSoldOut):
			return nil, status.Error(codes.FailedPrecondition, "market item is sold out")
		case errors.Is(err, viber.ErrExchangeInsufficientResource):
			return nil, status.Error(codes.ResourceExhausted, "insufficient market resource")
		default:
			return nil, err
		}
	}
	return response, nil
}

func marketRewards(ctx context.Context, rewardID string) ([]*pmaster.RewardResource, error) {
	rewards, err := viber.ParseAutoReward(rewardID)
	if err != nil {
		return nil, err
	}
	if rewards != nil {
		return rewards, nil
	}
	reward, err := viber.FindMasterReward(ctx, rewardID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, status.Error(codes.FailedPrecondition, "market reward not found")
		}
		return nil, err
	}
	return reward.GetResources(), nil
}

func findMarketItem(items []*papi.MarketItem, number int32) *papi.MarketItem {
	for _, item := range items {
		if item.GetNumber() == number {
			return item
		}
	}
	return nil
}
