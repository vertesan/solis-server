package servers

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
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
	"google.golang.org/protobuf/proto"
)

var (
	errExchangeLimitExceeded = errors.New("exchange limit exceeded")
)

type ExchangeServer struct {
	papi.UnimplementedExchangeServer
}

func (s *ExchangeServer) List(ctx context.Context, _ *papi.Empty) (*papi.ExchangeListResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.ExchangeListResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
		purchase, err := findExchangePurchaseOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		catalog, err := opsolis.FindAllExchangeBooths(ctx)
		if err != nil {
			return nil, err
		}

		userBooths, err := opuser.FindUserExchangeBooth(ctx, suid)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		if userBooths == nil {
			userBooths = &umodel.UserExchangeBooth{SUID: suid}
		}
		allowed := viber.StringSet(userBooths.BoothIds)
		for _, definition := range catalog {
			if _, ok := allowed[definition.GetId()]; !ok {
				allowed[definition.GetId()] = struct{}{}
				userBooths.BoothIds = append(userBooths.BoothIds, definition.GetId())
			}
		}
		if err := opuser.UpsertUserExchangeBooth(ctx, userBooths); err != nil {
			return nil, err
		}
		evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now)
		booths := make([]*papi.ExchangeBooth, 0, len(catalog))
		for _, definition := range catalog {
			if _, ok := allowed[definition.Id]; !ok {
				continue
			}
			if definition.EndTime > 0 && definition.EndTime <= now {
				continue
			}
			booth, visible, err := renderExchangeBooth(evaluator, definition, purchase)
			if err != nil {
				return nil, err
			}
			if visible {
				booths = append(booths, booth)
			}
		}
		response = &papi.ExchangeListResponse{Booths: booths, CommonResponse: viber.NewCommonResponse(nil)}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (s *ExchangeServer) Execute(ctx context.Context, req *papi.ExchangeExecuteRequest) (*papi.ExchangeExecuteResponse, error) {
	if req.GetExchangeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "exchangeId is required")
	}
	if req.GetCount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "count must be positive")
	}

	suid := viber.GetSuid(ctx).String()
	var response *papi.ExchangeExecuteResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
		definition, err := opsolis.FindExchangeBoothByExchangeId(ctx, req.GetExchangeId())
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.NotFound, "exchange item not found")
			}
			return nil, err
		}
		exchangeItem := findExchangeItem(definition, req.GetExchangeId())
		if exchangeItem == nil {
			return nil, status.Error(codes.NotFound, "exchange item not found")
		}
		if definition.GetEndTime() > 0 && definition.GetEndTime() <= now {
			return nil, status.Error(codes.FailedPrecondition, "exchange booth has ended")
		}

		evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now)
		boothVisible, err := evaluator.Satisfied(definition.GetViewConditionId())
		if err != nil {
			return nil, err
		}
		boothUnlocked, err := evaluator.Satisfied(definition.GetUnlockConditionId())
		if err != nil {
			return nil, err
		}
		itemVisible, err := evaluator.Satisfied(exchangeItem.GetViewConditionId())
		if err != nil {
			return nil, err
		}
		itemUnlocked, err := evaluator.Satisfied(exchangeItem.GetUnlockConditionId())
		if err != nil {
			return nil, err
		}
		if !boothVisible || !boothUnlocked || !itemVisible || !itemUnlocked {
			return nil, status.Error(codes.PermissionDenied, "exchange item is locked")
		}
		if exchangeItem.GetRequiredResourceAmount() <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "exchange cost must be positive")
		}
		if exchangeItem.GetRequiredResourceAmount() > math.MaxInt64/int64(req.GetCount()) {
			return nil, status.Error(codes.InvalidArgument, "exchange amount overflows")
		}
		rewards, err := resolveExchangeRewards(ctx, exchangeItem.GetRewardId(), req.GetCount())
		if err != nil {
			return nil, err
		}
		if err := viber.ValidateExchangeResources(definition.GetRequiredResourceType(), rewards); err != nil {
			return nil, err
		}

		purchase, err := findExchangePurchaseOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		resetPurchaseCountIfNeeded(purchase, exchangeItem)
		purchased := purchase.PurchaseCounts[exchangeItem.Id]
		nextPurchaseCount := int64(purchased) + int64(req.GetCount())
		if purchased < 0 || nextPurchaseCount > math.MaxInt32 {
			return nil, status.Error(codes.FailedPrecondition, "exchange purchase count is invalid")
		}
		if exchangeItem.ExchangeLimit > 0 && nextPurchaseCount > int64(exchangeItem.ExchangeLimit) {
			return nil, errExchangeLimitExceeded
		}

		updated := &papi.UpdatedData{}
		cost := exchangeItem.RequiredResourceAmount * int64(req.GetCount())
		if err := viber.ConsumeExchangeResource(ctx, suid, definition.RequiredResourceType, definition.RequiredResourceId, cost, updated); err != nil {
			return nil, err
		}

		results := make([]*ptransaction.RewardResult, 0, len(rewards))
		for _, reward := range rewards {
			result, err := viber.GrantExchangeReward(ctx, suid, reward, updated)
			if err != nil {
				return nil, err
			}
			results = append(results, result)
		}

		purchase.PurchaseCounts[exchangeItem.Id] = int32(nextPurchaseCount)
		if err := opuser.UpsertUserExchangePurchase(ctx, purchase); err != nil {
			return nil, err
		}

		postEvaluator := viber.NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
		booth, _, err := renderExchangeBoothAfterExecute(postEvaluator, definition, purchase, exchangeItem.Id)
		if err != nil {
			return nil, err
		}
		afterItem := findExchangeItem(booth, exchangeItem.Id)
		if afterItem == nil {
			return nil, status.Error(codes.Internal, "executed exchange item is missing from the post-state")
		}
		response = &papi.ExchangeExecuteResponse{
			Results:           results,
			AfterExchangeItem: afterItem,
			Booth:             booth,
			CommonResponse:    viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		switch {
		case mongo.IsDuplicateKeyError(err):
			return nil, status.Error(codes.FailedPrecondition, "exchange purchase state changed concurrently")
		case errors.Is(err, viber.ErrExchangeInsufficientResource):
			return nil, status.Error(codes.ResourceExhausted, "insufficient exchange resource")
		case errors.Is(err, errExchangeLimitExceeded):
			return nil, status.Error(codes.FailedPrecondition, "exchange limit exceeded")
		default:
			return nil, err
		}
	}
	return response, nil
}

func renderExchangeBoothAfterExecute(
	evaluator *viber.CommerceConditionEvaluator,
	definition *papi.ExchangeBooth,
	purchase *umodel.UserExchangePurchase,
	executedExchangeID string,
) (*papi.ExchangeBooth, bool, error) {
	visible, err := evaluator.Satisfied(definition.GetViewConditionId())
	if err != nil {
		return nil, false, err
	}
	unlocked, err := evaluator.Satisfied(definition.GetUnlockConditionId())
	if err != nil {
		return nil, false, err
	}

	booth := proto.Clone(definition).(*papi.ExchangeBooth)
	booth.Unlocked = unlocked
	applyExchangeStock(booth, purchase)
	items := make([]*papi.ExchangeItem, 0, len(booth.Exchanges))
	for _, item := range booth.Exchanges {
		itemVisible, err := evaluator.Satisfied(item.GetViewConditionId())
		if err != nil {
			return nil, false, err
		}
		if !itemVisible && item.GetId() != executedExchangeID {
			continue
		}
		itemUnlocked, err := evaluator.Satisfied(item.GetUnlockConditionId())
		if err != nil {
			return nil, false, err
		}
		item.Unlocked = itemUnlocked
		items = append(items, item)
	}
	booth.Exchanges = items
	return booth, visible, nil
}

func renderExchangeBooth(
	evaluator *viber.CommerceConditionEvaluator,
	definition *papi.ExchangeBooth,
	purchase *umodel.UserExchangePurchase,
) (*papi.ExchangeBooth, bool, error) {
	visible, err := evaluator.Satisfied(definition.GetViewConditionId())
	if err != nil || !visible {
		return nil, visible, err
	}
	unlocked, err := evaluator.Satisfied(definition.GetUnlockConditionId())
	if err != nil {
		return nil, false, err
	}

	booth := proto.Clone(definition).(*papi.ExchangeBooth)
	booth.Unlocked = unlocked
	applyExchangeStock(booth, purchase)
	items := make([]*papi.ExchangeItem, 0, len(booth.Exchanges))
	for _, item := range booth.Exchanges {
		itemVisible, err := evaluator.Satisfied(item.GetViewConditionId())
		if err != nil {
			return nil, false, err
		}
		if !itemVisible {
			continue
		}
		itemUnlocked, err := evaluator.Satisfied(item.GetUnlockConditionId())
		if err != nil {
			return nil, false, err
		}
		item.Unlocked = itemUnlocked
		items = append(items, item)
	}
	booth.Exchanges = items
	return booth, true, nil
}

func findExchangePurchaseOrEmpty(ctx context.Context, suid string) (*umodel.UserExchangePurchase, error) {
	purchase, err := opuser.FindUserExchangePurchase(ctx, suid)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		purchase = &umodel.UserExchangePurchase{SUID: suid}
	}
	if purchase.PurchaseCounts == nil {
		purchase.PurchaseCounts = map[string]int32{}
	}
	if purchase.ResetTimes == nil {
		purchase.ResetTimes = map[string]int64{}
	}
	return purchase, nil
}

func findExchangeItem(booth *papi.ExchangeBooth, exchangeId string) *papi.ExchangeItem {
	for _, item := range booth.GetExchanges() {
		if item.GetId() == exchangeId {
			return item
		}
	}
	return nil
}

func applyExchangeStock(booth *papi.ExchangeBooth, purchase *umodel.UserExchangePurchase) {
	for _, item := range booth.Exchanges {
		resetPurchaseCountIfNeeded(purchase, item)
		if item.ExchangeLimit > 0 {
			item.LeftCount = max(item.ExchangeLimit-purchase.PurchaseCounts[item.Id], 0)
		} else {
			item.LeftCount = 0
		}
	}
}

func resetPurchaseCountIfNeeded(purchase *umodel.UserExchangePurchase, item *papi.ExchangeItem) {
	if item.NextResetTime <= 0 || item.ResetTimingType == penum.ResetTimingType_ResetTimingType_Unknown ||
		item.ResetTimingType == penum.ResetTimingType_ResetTimingType_Never {
		return
	}

	resetTime := purchase.ResetTimes[item.Id]
	if resetTime == 0 || item.NextResetTime > resetTime {
		purchase.PurchaseCounts[item.Id] = 0
		resetTime = item.NextResetTime
		purchase.ResetTimes[item.Id] = resetTime
	}
	if now := time.Now().UnixMilli(); resetTime <= now {
		purchase.PurchaseCounts[item.Id] = 0
		purchase.ResetTimes[item.Id] = nextExchangeResetTime(resetTime, item.ResetTimingType, now)
	}
}

func nextExchangeResetTime(resetTime int64, timing penum.ResetTimingType, now int64) int64 {
	switch timing {
	case penum.ResetTimingType_ResetTimingType_Daily:
		const period = int64(24 * time.Hour / time.Millisecond)
		return resetTime + ((now-resetTime)/period+1)*period
	case penum.ResetTimingType_ResetTimingType_Weekly:
		const period = int64(7 * 24 * time.Hour / time.Millisecond)
		return resetTime + ((now-resetTime)/period+1)*period
	case penum.ResetTimingType_ResetTimingType_Monthly:
		reset := time.UnixMilli(resetTime).In(jstZone)
		current := time.UnixMilli(now).In(jstZone)
		months := (current.Year()-reset.Year())*12 + int(current.Month()-reset.Month())
		next := reset.AddDate(0, months, 0)
		if !next.After(current) {
			next = next.AddDate(0, 1, 0)
		}
		return next.UnixMilli()
	default:
		return resetTime
	}
}

func resolveExchangeRewards(ctx context.Context, rewardId string, count int32) ([]*pmaster.RewardResource, error) {
	resources, err := viber.ParseAutoReward(rewardId)
	if err != nil {
		return nil, err
	}
	if resources == nil {
		reward, err := findMasterExchangeReward(ctx, rewardId)
		if err != nil {
			return nil, err
		}
		resources = reward.Resources
	}
	resolved := make([]*pmaster.RewardResource, 0, len(resources))
	for _, resource := range resources {
		amount := int64(resource.Amount) * int64(count)
		if amount <= 0 || amount > math.MaxInt32 {
			return nil, status.Error(codes.InvalidArgument, "invalid reward amount")
		}
		resolved = append(resolved, &pmaster.RewardResource{
			ResourceType: resource.ResourceType,
			ResourceId:   resource.ResourceId,
			Amount:       int32(amount),
		})
	}
	return resolved, nil
}

func findMasterExchangeReward(ctx context.Context, rewardId string) (*pmaster.Reward, error) {
	reward, err := viber.FindMasterReward(ctx, rewardId)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("reward %q is missing from master data", rewardId))
		}
		return nil, err
	}
	return reward, nil
}
