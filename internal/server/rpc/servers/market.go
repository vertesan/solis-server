package servers

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
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
	errMarketItemSoldOut    = errors.New("market item is sold out")
	errMarketFreeResetUsed  = errors.New("market free reset is unavailable")
	errMarketPaidResetLimit = errors.New("market paid reset limit reached")
)

type MarketServer struct {
	papi.UnimplementedMarketServer
}

func (s *MarketServer) ListItem(ctx context.Context, _ *papi.Empty) (*papi.MarketListItemResponse, error) {
	suid := viber.GetSuid(ctx).String()
	now := time.Now()
	catalog, err := viber.LoadMarketCatalog()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load market catalog: %v", err)
	}

	var response *papi.MarketListItemResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		state, err := findAndRefreshMarketState(ctx, suid, now, catalog)
		if err != nil {
			return nil, err
		}
		response = &papi.MarketListItemResponse{
			MarketItems:              cloneMarketItems(state.MarketItems),
			NextResetTime:            viber.NextJSTHalfDayStart(now).UnixMilli(),
			FreeResetRemainCount:     marketFreeResetRemainCount(state, now),
			ResetRemainCount:         marketPaidResetRemainCount(state, catalog),
			RequiredResetStoneAmount: catalog.RequiredResetStoneAmount,
			CommonResponse:           viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (s *MarketServer) Reset(ctx context.Context, req *papi.MarketResetRequest) (*papi.MarketResetResponse, error) {
	suid := viber.GetSuid(ctx).String()
	now := time.Now()
	catalog, err := viber.LoadMarketCatalog()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load market catalog: %v", err)
	}

	var response *papi.MarketResetResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		state, err := findAndRefreshMarketState(ctx, suid, now, catalog)
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		if req.GetIsFree() {
			boundary := viber.JSTHalfDayStart(now).UnixMilli()
			if state.FreeResetBoundaryTime == boundary {
				return nil, errMarketFreeResetUsed
			}
			state.FreeResetBoundaryTime = boundary
		} else {
			if state.PaidResetCount >= catalog.PaidResetLimit {
				return nil, errMarketPaidResetLimit
			}
			if err := viber.ConsumeExchangeResource(
				ctx,
				suid,
				penum.ResourceType_ResourceType_Stone,
				"",
				int64(catalog.RequiredResetStoneAmount),
				updated,
			); err != nil {
				return nil, err
			}
			state.PaidResetCount++
		}

		items, err := generateMarketItems(ctx, suid, catalog, now)
		if err != nil {
			return nil, err
		}
		state.MarketItems = items
		if err := opuser.UpsertUserMarketState(ctx, state); err != nil {
			return nil, err
		}
		response = &papi.MarketResetResponse{
			MarketItems:              cloneMarketItems(items),
			FreeResetRemainCount:     marketFreeResetRemainCount(state, now),
			ResetRemainCount:         marketPaidResetRemainCount(state, catalog),
			RequiredResetStoneAmount: catalog.RequiredResetStoneAmount,
			CommonResponse:           viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		switch {
		case errors.Is(err, errMarketFreeResetUsed):
			return nil, status.Error(codes.FailedPrecondition, "free market reset is unavailable")
		case errors.Is(err, errMarketPaidResetLimit):
			return nil, status.Error(codes.FailedPrecondition, "market reset limit reached")
		case errors.Is(err, viber.ErrExchangeInsufficientResource):
			return nil, status.Error(codes.ResourceExhausted, "insufficient stones")
		default:
			return nil, err
		}
	}
	return response, nil
}

func (s *MarketServer) Purchase(ctx context.Context, req *papi.MarketPurchaseRequest) (*papi.MarketPurchaseResponse, error) {
	if req.GetNumber() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "number must be positive")
	}
	suid := viber.GetSuid(ctx).String()
	now := time.Now()
	catalog, err := viber.LoadMarketCatalog()
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
		unlocked := item.GetUnlockConditionId() == catalog.DiscountUnlockConditionID && viber.IsJSTMarketDiscountDay(now)
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

func findAndRefreshMarketState(
	ctx context.Context,
	suid string,
	now time.Time,
	catalog *viber.MarketCatalog,
) (*umodel.UserMarketState, error) {
	state, err := opuser.FindUserMarketState(ctx, suid)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		state = &umodel.UserMarketState{SUID: suid}
	}
	changed := false
	boundary := viber.JSTHalfDayStart(now)
	if state.InventoryBoundaryTime != boundary.UnixMilli() || len(state.MarketItems) == 0 {
		items, err := generateMarketItems(ctx, suid, catalog, now)
		if err != nil {
			return nil, err
		}
		state.MarketItems = items
		state.InventoryBoundaryTime = boundary.UnixMilli()
		changed = true
	}
	dayStart := boundary
	if boundary.Hour() == 12 {
		dayStart = boundary.Add(-12 * time.Hour)
	}
	if state.PaidResetDayTime != dayStart.UnixMilli() {
		state.PaidResetDayTime = dayStart.UnixMilli()
		state.PaidResetCount = 0
		changed = true
	}
	if changed {
		if err := opuser.UpsertUserMarketState(ctx, state); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func generateMarketItems(ctx context.Context, suid string, catalog *viber.MarketCatalog, now time.Time) ([]*papi.MarketItem, error) {
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, err
	}
	tier := marketTierForLevel(catalog, user.GetManagerLevel())
	if tier == nil {
		return nil, status.Error(codes.FailedPrecondition, "market catalog has no manager-level tier")
	}

	evaluator := viber.NewCommerceConditionEvaluator(ctx, suid, now.UnixMilli())
	discountDay, err := evaluator.Satisfied(catalog.DiscountDayConditionID)
	if err != nil {
		return nil, err
	}
	discountDay = discountDay || viber.IsJSTMarketDiscountDay(now)
	offset := catalog.RegularNumberOffset
	conditionID := catalog.RegularUnlockConditionID
	if discountDay {
		offset = catalog.DiscountNumberOffset
		conditionID = catalog.DiscountUnlockConditionID
	}

	accessoriesByRarity := make(map[int32][]string)
	items := make([]*papi.MarketItem, 0, len(tier.Slots))
	for _, slot := range tier.Slots {
		variant, err := chooseMarketSlotVariant(slot)
		if err != nil {
			return nil, err
		}
		resourceType, err := marketResourceType(variant.ResourceType)
		if err != nil {
			return nil, err
		}
		rewardID, err := marketRewardID(ctx, variant, accessoriesByRarity)
		if err != nil {
			return nil, err
		}
		discountRate := catalog.DiscountDayRatePermil
		if !discountDay {
			discountRate, err = randomInt32(catalog.RegularDiscountRatePermils)
			if err != nil {
				return nil, err
			}
		}
		if discountRate < 0 || discountRate > 1000 || variant.BaseAmount <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "market catalog has an invalid price")
		}
		discountAmount := int64(variant.BaseAmount) * int64(1000-discountRate) / 1000
		if discountAmount <= 0 || discountAmount > math.MaxInt32 {
			return nil, status.Error(codes.FailedPrecondition, "market discounted price is invalid")
		}
		items = append(items, &papi.MarketItem{
			Number:                  offset + slot.Number,
			ConsumptionResourceType: resourceType,
			BaseAmount:              variant.BaseAmount,
			DiscountAmount:          int32(discountAmount),
			RewardId:                rewardID,
			DiscountRatePermil:      discountRate,
			SoldOut:                 false,
			UnlockConditionId:       conditionID,
		})
	}
	return items, nil
}

func chooseMarketSlotVariant(slot viber.MarketSlot) (viber.MarketSlotVariant, error) {
	if len(slot.Variants) == 0 {
		return slot.MarketSlotVariant, nil
	}
	index, err := viber.CryptoRandomIndex(len(slot.Variants))
	if err != nil {
		return viber.MarketSlotVariant{}, err
	}
	return slot.Variants[index], nil
}

func marketTierForLevel(catalog *viber.MarketCatalog, level int32) *viber.MarketTier {
	for i := range catalog.Tiers {
		tier := &catalog.Tiers[i]
		if tier.MaxManagerLevel == 0 || level <= tier.MaxManagerLevel {
			return tier
		}
	}
	return nil
}

func marketResourceType(value string) (penum.ResourceType, error) {
	switch value {
	case "gold":
		return penum.ResourceType_ResourceType_Gold, nil
	case "stone":
		return penum.ResourceType_ResourceType_Stone, nil
	default:
		return penum.ResourceType_ResourceType_Unknown, status.Errorf(codes.FailedPrecondition, "unsupported market resource %q", value)
	}
}

func marketRewardID(ctx context.Context, slot viber.MarketSlotVariant, accessoriesByRarity map[int32][]string) (string, error) {
	var candidates []string
	switch slot.RewardKind {
	case "item":
		for _, itemID := range slot.RewardIDs {
			if _, err := opmaster.FindItem(ctx, itemID); err == nil {
				candidates = append(candidates, itemID)
			} else if !errors.Is(err, mongo.ErrNoDocuments) {
				return "", err
			}
		}
	case "accessory":
		candidates = accessoriesByRarity[slot.AccessoryRarity]
		if candidates == nil {
			accessories, err := opmaster.FindMarketAccessoriesByRarity(ctx, slot.AccessoryRarity)
			if err != nil {
				return "", err
			}
			for _, accessory := range accessories {
				candidates = append(candidates, accessory.GetId())
			}
			accessoriesByRarity[slot.AccessoryRarity] = candidates
		}
	default:
		return "", status.Errorf(codes.FailedPrecondition, "unsupported market reward %q", slot.RewardKind)
	}
	if len(candidates) == 0 || slot.RewardAmount <= 0 || slot.RewardAmount > math.MaxInt32 {
		return "", status.Error(codes.FailedPrecondition, "market reward catalog has no valid candidates")
	}
	index, err := viber.CryptoRandomIndex(len(candidates))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("rd_auto;%s;%s;%d", slot.RewardKind, candidates[index], slot.RewardAmount), nil
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

func marketFreeResetRemainCount(state *umodel.UserMarketState, now time.Time) int32 {
	if state.FreeResetBoundaryTime == viber.JSTHalfDayStart(now).UnixMilli() {
		return 0
	}
	return 1
}

func marketPaidResetRemainCount(state *umodel.UserMarketState, catalog *viber.MarketCatalog) int32 {
	return max(0, catalog.PaidResetLimit-state.PaidResetCount)
}

func findMarketItem(items []*papi.MarketItem, number int32) *papi.MarketItem {
	for _, item := range items {
		if item.GetNumber() == number {
			return item
		}
	}
	return nil
}

func cloneMarketItems(items []*papi.MarketItem) []*papi.MarketItem {
	clones := make([]*papi.MarketItem, 0, len(items))
	for _, item := range items {
		clones = append(clones, proto.Clone(item).(*papi.MarketItem))
	}
	return clones
}

func randomInt32(values []int32) (int32, error) {
	if len(values) == 0 {
		return 0, status.Error(codes.FailedPrecondition, "market discount catalog is empty")
	}
	index, err := viber.CryptoRandomIndex(len(values))
	if err != nil {
		return 0, err
	}
	return values[index], nil
}
