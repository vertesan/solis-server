package servers

import (
	"context"
	"crypto/rand"
	"errors"
	"math"
	"math/big"
	"slices"
	"sort"
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

const (
	gachaHistoryLimit       = 300
	premiumFreeTicketAmount = 10
	gachaProbabilityTotal   = 100000
)

type GachaServer struct {
	papi.UnimplementedGachaServer
}

func (s *GachaServer) List(ctx context.Context, _ *papi.Empty) (*papi.GachaListResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.GachaListResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		now := time.Now().UnixMilli()
		visible, err := viber.ReconcileCurrentGachaInfos(ctx, suid, now)
		if err != nil {
			return nil, err
		}
		response = &papi.GachaListResponse{
			Gachas:         visible,
			CommonResponse: viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *GachaServer) Draw(ctx context.Context, req *papi.GachaDrawRequest) (*papi.GachaDrawResponse, error) {
	if req.GetGachaButtonId() == "" {
		return nil, status.Error(codes.InvalidArgument, "gachaButtonId is required")
	}
	if req.GetExecCount() <= 0 || req.GetExecCount() > 1000 {
		return nil, status.Error(codes.InvalidArgument, "execCount must be between 1 and 1000")
	}

	suid := viber.GetSuid(ctx).String()
	var response *papi.GachaDrawResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		catalog, err := viber.LoadGachaMasterCatalog(ctx)
		if err != nil {
			return nil, err
		}
		button := catalog.Buttons[req.GetGachaButtonId()]
		if button == nil {
			return nil, status.Error(codes.NotFound, "gacha button not found")
		}
		masterGacha := catalog.Gachas[button.GetGachaId()]
		if masterGacha == nil {
			return nil, status.Error(codes.FailedPrecondition, "gacha button has no gacha")
		}
		now := time.Now().UnixMilli()
		state, err := viber.CurrentGachaInfo(ctx, suid, catalog, masterGacha, now)
		if err != nil {
			return nil, err
		}
		if !state.GetUnlocked() {
			return nil, status.Error(codes.PermissionDenied, "gacha is locked")
		}
		executionCount := req.GetExecCount()
		if masterGacha.GetGachaType() == penum.GachaType_GachaType_Continuous {
			// Captured continuous requests send the accumulated client draw
			// count here. Each invocation still executes the button once.
			executionCount = 1
		}

		buttonState := findGachaButtonInfo(state, button.GetId())
		if buttonState == nil {
			return nil, status.Error(codes.FailedPrecondition, "gacha button state is missing")
		}
		viber.ResetGachaDailyCount(buttonState, now)
		if err := validateGachaButtonLimit(button, buttonState, executionCount); err != nil {
			return nil, err
		}

		updated := &papi.UpdatedData{}
		consumptionID, err := gachaConsumptionID(button, buttonState, executionCount)
		if err != nil {
			return nil, err
		}
		consumptionResults, err := consumeGacha(ctx, suid, consumptionID, executionCount, updated)
		if err != nil {
			return nil, err
		}
		pool, err := buildGachaPool(ctx, masterGacha, state, now)
		if err != nil {
			return nil, err
		}
		if len(pool) == 0 {
			return nil, status.Error(codes.FailedPrecondition, "gacha reward pool is empty")
		}
		drawCount := int64(button.GetDrawCount()) * int64(executionCount)
		if button.GetDrawCount() <= 0 || drawCount > 10000 {
			return nil, status.Error(codes.FailedPrecondition, "gacha draw count is invalid")
		}

		rewards := make([]*ptransaction.Reward, 0, drawCount)
		resultList := make([]*ptransaction.RewardResult, 0, drawCount)
		drawResultList := make([]*ptransaction.RewardResult, 0, 2)
		overflowGiftBatch := viber.NewGachaOverflowGiftBatch(now)
		cardRewards := make([]*ptransaction.Reward, 0)
		pickupRewards := make([]*ptransaction.Reward, 0)
		histories := make([]*papi.GachaHistory, 0, drawCount)
		premiumRemaining := int32(0)
		premiumRewardResourceID := ""
		premiumGuaranteeCount := int32(0)
		drawRules, err := viber.LoadGachaRules()
		if err != nil {
			return nil, err
		}
		if masterGacha.GetGachaType() == penum.GachaType_GachaType_Premium {
			if state.GetPremiumInfo() == nil {
				return nil, status.Error(codes.FailedPrecondition, "premium gacha state is missing")
			}
			premiumGuaranteeCount = drawRules.PremiumGuaranteeDrawCount
			premiumRemaining = state.GetPremiumInfo().GetFixedPremiumRewardRequiredDrawCount()
			if premiumRemaining <= 0 || premiumRemaining > premiumGuaranteeCount {
				premiumRemaining = premiumGuaranteeCount
			}
			if len(pool) == 0 {
				return nil, status.Error(codes.FailedPrecondition, "premium gacha reward pool is empty")
			}
			premiumRewardResourceID = pool[0].GetResourceId()
		}
		guaranteedPool, cardRarities, err := guaranteedGachaBatchPool(
			ctx, pool, button.GetDrawCount(), drawRules.GuaranteedCardBatchSize, drawRules.GuaranteedCardMinimumRarity,
		)
		if err != nil {
			return nil, err
		}
		batchHasGuaranteedRarity := false
		for drawIndex := int64(0); drawIndex < drawCount; drawIndex++ {
			var selected *pmaster.RewardResource
			if premiumRemaining == 1 {
				selected = proto.Clone(pool[0]).(*pmaster.RewardResource)
			} else if len(guaranteedPool) > 0 &&
				(drawIndex+1)%int64(drawRules.GuaranteedCardBatchSize) == 0 &&
				!batchHasGuaranteedRarity {
				selected, err = selectGachaReward(ctx, guaranteedPool, masterGacha, state.GetSelectedCardIds())
			} else {
				selected, err = selectGachaReward(ctx, pool, masterGacha, state.GetSelectedCardIds())
			}
			if err != nil {
				return nil, err
			}
			if premiumRemaining > 0 {
				premiumRemaining--
				if selected.GetResourceId() == premiumRewardResourceID {
					premiumRemaining = premiumGuaranteeCount
				}
			}
			if len(guaranteedPool) > 0 && cardRarities[selected.GetResourceId()] >= drawRules.GuaranteedCardMinimumRarity {
				batchHasGuaranteedRarity = true
			}
			if len(guaranteedPool) > 0 && (drawIndex+1)%int64(drawRules.GuaranteedCardBatchSize) == 0 {
				batchHasGuaranteedRarity = false
			}
			reward := &ptransaction.Reward{
				ResourceType: selected.GetResourceType(),
				ResourceId:   selected.GetResourceId(),
				Amount:       int64(selected.GetAmount()),
			}
			rewards = append(rewards, reward)
			if slices.Contains(masterGacha.GetPickupCardIds(), selected.GetResourceId()) ||
				slices.Contains(state.GetSelectedCardIds(), selected.GetResourceId()) {
				pickupRewards = append(pickupRewards, proto.Clone(reward).(*ptransaction.Reward))
			}
			results, err := viber.GrantGachaDrawReward(ctx, suid, selected, overflowGiftBatch, updated)
			if err != nil {
				return nil, err
			}
			resultList = append(resultList, results...)
			drawCardRewards, drawCardResults, err := grantGachaCardRewards(
				ctx, suid, masterGacha, selected, overflowGiftBatch, updated,
			)
			if err != nil {
				return nil, err
			}
			cardRewards = append(cardRewards, drawCardRewards...)
			resultList = append(resultList, drawCardResults...)
			histories = append(histories, &papi.GachaHistory{
				GachaType:    masterGacha.GetGachaType(),
				DrawTime:     now,
				GachaId:      masterGacha.GetId(),
				ResourceType: selected.GetResourceType(),
				ResourceId:   selected.GetResourceId(),
				Amount:       selected.GetAmount(),
			})
		}
		if state.GetPremiumInfo() != nil {
			state.PremiumInfo.FixedPremiumRewardRequiredDrawCount = premiumRemaining
		}
		if masterGacha.GetGachaType() == penum.GachaType_GachaType_Continuous {
			if state.GetContinuousInfo() == nil || drawCount > math.MaxInt32-int64(state.GetContinuousInfo().GetTotalDrawCount()) {
				return nil, status.Error(codes.FailedPrecondition, "continuous gacha draw count overflows")
			}
			state.ContinuousInfo.TotalDrawCount += int32(drawCount)
		}
		exchangeResults, err := viber.GrantGachaExchangeResource(
			ctx,
			suid,
			masterGacha.GetExchangeResourceType(),
			masterGacha.GetExchangeResourceId(),
			drawCount,
			viber.GachaTermEnd(ctx, masterGacha.GetViewConditionId()),
			updated,
		)
		if err != nil {
			return nil, err
		}
		drawResultList = append(drawResultList, exchangeResults...)

		bonusExpiration, err := viber.GachaButtonBonusExpiration(ctx, masterGacha.GetViewConditionId())
		if err != nil {
			return nil, err
		}
		_, bonusResults, err := grantGachaButtonBonus(
			ctx, suid, button, buttonState.GetTotalCount(), executionCount, bonusExpiration, overflowGiftBatch, updated,
		)
		if err != nil {
			return nil, err
		}
		drawResultList = append(drawResultList, bonusResults...)
		if err := overflowGiftBatch.Flush(ctx, suid, updated); err != nil {
			return nil, err
		}
		buttonState.TodayCount += executionCount
		buttonState.TotalCount += executionCount
		buttonState.DrawTime = now
		if err := opuser.UpsertUserGachaInfo(ctx, suid, state); err != nil {
			return nil, err
		}
		if err := appendGachaHistories(ctx, suid, masterGacha.GetGachaType(), histories); err != nil {
			return nil, err
		}
		updated.GachaButtons = append(updated.GachaButtons, &ptransaction.UserGachaButton{
			GachaButtonId: buttonState.GetId(),
			DrawTime:      buttonState.GetDrawTime(),
			TodayCount:    buttonState.GetTodayCount(),
			TotalCount:    buttonState.GetTotalCount(),
		})
		resultList, err = viber.AggregateGachaRewardResults(resultList)
		if err != nil {
			return nil, err
		}
		drawResultList, err = viber.AggregateGachaRewardResults(drawResultList)
		if err != nil {
			return nil, err
		}
		viber.CanonicalizeGachaUpdatedData(updated)

		response = &papi.GachaDrawResponse{
			Button:             proto.Clone(buttonState).(*papi.GachaButtonInfo),
			Rewards:            rewards,
			ConsumptionResults: consumptionResults,
			RewardResults:      resultList,
			DrawRewardResults:  drawResultList,
			BonusRewards:       []*ptransaction.Reward{},
			CardRewards:        cardRewards,
			PickupRewards:      pickupRewards,
			CommonResponse:     viber.NewCommonResponse(updated),
		}
		if masterGacha.GetGachaType() == penum.GachaType_GachaType_Continuous {
			response.ContinuousResult = &papi.GachaContinuousResult{
				TotalDrawCount: state.GetContinuousInfo().GetTotalDrawCount(),
				CardIds:        append([]string(nil), state.GetContinuousInfo().GetCardIds()...),
				ResultType:     penum.GachaContinuousResultType_GachaContinuousResultType_Continue,
			}
		}
		return nil, nil
	})
	if err != nil {
		if errors.Is(err, viber.ErrExchangeInsufficientResource) {
			return nil, status.Error(codes.ResourceExhausted, "insufficient gacha resource")
		}
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *GachaServer) Probability(ctx context.Context, req *papi.GachaProbabilityRequest) (*papi.GachaProbabilityResponse, error) {
	if req.GetGachaId() == "" {
		return nil, status.Error(codes.InvalidArgument, "gachaId is required")
	}
	suid := viber.GetSuid(ctx).String()
	catalog, err := viber.LoadGachaMasterCatalog(ctx)
	if err != nil {
		return nil, err
	}
	masterGacha := catalog.Gachas[req.GetGachaId()]
	if masterGacha == nil {
		return nil, status.Error(codes.NotFound, "gacha not found")
	}
	now := time.Now().UnixMilli()
	state, err := viber.CurrentGachaInfo(ctx, suid, catalog, masterGacha, now)
	if err != nil {
		return nil, err
	}
	if !state.GetUnlocked() {
		return nil, status.Error(codes.PermissionDenied, "gacha is locked")
	}
	pool, err := buildGachaPool(ctx, masterGacha, state, now)
	if err != nil {
		return nil, err
	}
	probabilities, rarityProbabilities, err := calculateGachaProbabilities(ctx, masterGacha, state, pool)
	if err != nil {
		return nil, err
	}
	return &papi.GachaProbabilityResponse{
		RarityProbabilityInfos: rarityProbabilities,
		ProbabilityInfos:       probabilities,
		CommonResponse:         viber.NewCommonResponse(nil),
	}, nil
}

func (s *GachaServer) History(ctx context.Context, req *papi.GachaHistoryRequest) (*papi.GachaHistoryResponse, error) {
	if req.GetGachaType() == penum.GachaType_GachaType_Unknown {
		return nil, status.Error(codes.InvalidArgument, "gachaType is required")
	}
	history, err := opuser.FindUserGachaHistory(ctx, viber.GetSuid(ctx).String(), req.GetGachaType())
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return &papi.GachaHistoryResponse{Histories: []*papi.GachaHistory{}, CommonResponse: viber.NewCommonResponse(nil)}, nil
		}
		return nil, err
	}
	return &papi.GachaHistoryResponse{
		Histories:      history.Histories,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *GachaServer) SelectCard(ctx context.Context, req *papi.GachaSelectCardRequest) (*papi.GachaSelectCardResponse, error) {
	if req.GetGachaId() == "" {
		return nil, status.Error(codes.InvalidArgument, "gachaId is required")
	}
	suid := viber.GetSuid(ctx).String()
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		catalog, err := viber.LoadGachaMasterCatalog(ctx)
		if err != nil {
			return nil, err
		}
		masterGacha := catalog.Gachas[req.GetGachaId()]
		if masterGacha == nil {
			return nil, status.Error(codes.NotFound, "gacha not found")
		}
		if int32(len(req.GetSelectedCardIds())) != masterGacha.GetSelectPickupAmount() {
			return nil, status.Errorf(codes.InvalidArgument, "exactly %d cards must be selected", masterGacha.GetSelectPickupAmount())
		}
		allowed := viber.StringSet(masterGacha.GetSelectCardIds())
		seen := make(map[string]struct{}, len(req.GetSelectedCardIds()))
		for _, cardID := range req.GetSelectedCardIds() {
			if _, ok := allowed[cardID]; !ok {
				return nil, status.Errorf(codes.InvalidArgument, "card %q cannot be selected", cardID)
			}
			if _, ok := seen[cardID]; ok {
				return nil, status.Errorf(codes.InvalidArgument, "card %q is selected more than once", cardID)
			}
			seen[cardID] = struct{}{}
		}
		now := time.Now().UnixMilli()
		state, err := viber.CurrentGachaInfo(ctx, suid, catalog, masterGacha, now)
		if err != nil {
			return nil, err
		}
		if !state.GetUnlocked() {
			return nil, status.Error(codes.PermissionDenied, "gacha is locked")
		}
		state.SelectedCardIds = append([]string(nil), req.GetSelectedCardIds()...)
		return nil, opuser.UpsertUserGachaInfo(ctx, suid, state)
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return &papi.GachaSelectCardResponse{CommonResponse: viber.NewCommonResponse(nil)}, nil
}

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
		state, err := viber.CurrentGachaInfo(ctx, suid, catalog, masterGacha, now)
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
		expiresAt := viber.GachaTermEnd(ctx, masterGacha.GetViewConditionId())
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

func findGachaButtonInfo(info *papi.GachaInfo, id string) *papi.GachaButtonInfo {
	for _, button := range info.GetButtons() {
		if button.GetId() == id {
			return button
		}
	}
	return nil
}

func validateGachaButtonLimit(button *pmaster.GachaButton, state *papi.GachaButtonInfo, execCount int32) error {
	if button.GetLimitCount() <= 0 {
		return nil
	}
	var current int32
	switch button.GetLimitType() {
	case penum.GachaLimitType_GachaLimitType_Daily:
		current = state.GetTodayCount()
	case penum.GachaLimitType_GachaLimitType_Total:
		current = state.GetTotalCount()
	default:
		return status.Error(codes.FailedPrecondition, "gacha button has an invalid limit type")
	}
	if current < 0 || int64(current)+int64(execCount) > int64(button.GetLimitCount()) {
		return status.Error(codes.FailedPrecondition, "gacha button draw limit exceeded")
	}
	return nil
}

func gachaConsumptionID(button *pmaster.GachaButton, state *papi.GachaButtonInfo, execCount int32) (string, error) {
	if button.GetDiscountConsumptionId() == "" || button.GetDiscountCount() <= 0 {
		return button.GetConsumptionId(), nil
	}
	var current int32
	switch button.GetDiscountLimitType() {
	case penum.GachaLimitType_GachaLimitType_Daily:
		current = state.GetTodayCount()
	case penum.GachaLimitType_GachaLimitType_Total:
		current = state.GetTotalCount()
	default:
		return button.GetConsumptionId(), nil
	}
	if current < button.GetDiscountCount() {
		if int64(current)+int64(execCount) > int64(button.GetDiscountCount()) {
			return "", status.Error(codes.InvalidArgument, "execCount crosses the gacha discount boundary")
		}
		return button.GetDiscountConsumptionId(), nil
	}
	return button.GetConsumptionId(), nil
}

func consumeGacha(
	ctx context.Context,
	suid string,
	consumptionID string,
	execCount int32,
	updated *papi.UpdatedData,
) ([]*ptransaction.ConsumptionResult, error) {
	if consumptionID == "" {
		return []*ptransaction.ConsumptionResult{}, nil
	}
	consumption, err := opmaster.FindConsumption(ctx, consumptionID)
	if err != nil {
		return nil, err
	}
	results := make([]*ptransaction.ConsumptionResult, 0, len(consumption.GetResources()))
	for _, resource := range consumption.GetResources() {
		if resource.GetAmount() <= 0 || int64(resource.GetAmount()) > math.MaxInt64/int64(execCount) {
			return nil, status.Error(codes.FailedPrecondition, "gacha consumption amount is invalid")
		}
		resourceID := ""
		if len(resource.GetResourceIds()) > 0 {
			resourceID = resource.GetResourceIds()[0]
		}
		amount := int64(resource.GetAmount()) * int64(execCount)
		if resource.GetResourceType() == penum.ResourceType_ResourceType_Item {
			stackResults, err := viber.ConsumeExchangeItemStacks(ctx, suid, resourceID, amount, updated)
			if err != nil {
				return nil, err
			}
			results = append(results, stackResults...)
			continue
		}
		before, err := gachaResourceAmount(ctx, suid, resource.GetResourceType(), resourceID)
		if err != nil {
			return nil, err
		}
		if err := viber.ConsumeExchangeResource(
			ctx, suid, resource.GetResourceType(), resourceID, amount, updated,
		); err != nil {
			return nil, err
		}
		results = append(results, &ptransaction.ConsumptionResult{
			ResourceType: resource.GetResourceType(),
			ResourceId:   resourceID,
			Amount:       amount,
			BeforeAmount: before,
			AfterAmount:  before - amount,
		})
	}
	return results, nil
}

func gachaResourceAmount(
	ctx context.Context,
	suid string,
	resourceType penum.ResourceType,
	resourceID string,
) (int64, error) {
	switch resourceType {
	case penum.ResourceType_ResourceType_Item:
		items, err := opuser.FindUserItems(ctx, suid, resourceID)
		if err != nil {
			return 0, err
		}
		now := time.Now().UnixMilli()
		var amount int64
		for _, item := range items {
			if item.GetAmount() <= 0 || item.GetExpiredTime() > 0 && item.GetExpiredTime() <= now {
				continue
			}
			if item.GetAmount() > math.MaxInt64-amount {
				return 0, status.Error(codes.FailedPrecondition, "gacha resource amount overflows")
			}
			amount += item.GetAmount()
		}
		return amount, nil
	case penum.ResourceType_ResourceType_Stone, penum.ResourceType_ResourceType_PaidStone:
		balance, err := opuser.FindUserBalance(ctx, suid)
		if err != nil {
			return 0, err
		}
		if resourceType == penum.ResourceType_ResourceType_PaidStone {
			return int64(balance.GetPaidBalance()), nil
		}
		return int64(balance.GetFreeBalance()), nil
	default:
		return 0, status.Errorf(codes.Unimplemented, "gacha consumption resource type %s is unsupported", resourceType)
	}
}

func buildGachaPool(
	ctx context.Context,
	masterGacha *pmaster.Gacha,
	state *papi.GachaInfo,
	now int64,
) ([]*pmaster.RewardResource, error) {
	var pool []*pmaster.RewardResource
	switch masterGacha.GetGachaType() {
	case penum.GachaType_GachaType_Item:
		pool = transactionRewardsToMaster(state.GetItemGachaRewards())
		if len(pool) == 0 {
			return nil, status.Error(codes.FailedPrecondition, "item gacha has no configured reward pool")
		}
	case penum.GachaType_GachaType_ShowcaseToy:
		pool = transactionRewardsToMaster(state.GetShowcaseToyGachaRewards())
		if len(pool) == 0 {
			return nil, status.Error(codes.FailedPrecondition, "showcase-toy gacha has no configured reward pool")
		}
	case penum.GachaType_GachaType_Premium:
		weighted, err := viber.BuildPremiumGachaPool(ctx, masterGacha)
		if err != nil {
			return nil, err
		}
		pool = make([]*pmaster.RewardResource, 0, len(weighted))
		for _, entry := range weighted {
			pool = append(pool, proto.Clone(entry.Reward).(*pmaster.RewardResource))
		}
	}
	if len(pool) > 0 {
		return pool, nil
	}
	rules, err := viber.LoadGachaRules()
	if err != nil {
		return nil, err
	}
	if rarityPool := rules.GuaranteedRarityPool(masterGacha.GetId()); rarityPool != nil {
		if rarityPool.UseMasterCardRewards {
			return masterGachaCardRewardPool(ctx, masterGacha, rarityPool.MinimumRarity)
		}
		return fallbackCardPool(ctx, now, rarityPool.MinimumRarity)
	}
	return fallbackCardPool(ctx, now, 3)
}

func masterGachaCardRewardPool(ctx context.Context, masterGacha *pmaster.Gacha, minimumRarity int32) ([]*pmaster.RewardResource, error) {
	pool := make([]*pmaster.RewardResource, 0, len(masterGacha.GetCardRewards()))
	seen := make(map[string]struct{}, len(masterGacha.GetCardRewards()))
	for _, definition := range masterGacha.GetCardRewards() {
		cardID := definition.GetCardId()
		if cardID == "" {
			return nil, status.Error(codes.FailedPrecondition, "master-derived gacha pool contains an empty card ID")
		}
		if _, exists := seen[cardID]; exists {
			continue
		}
		card, err := opmaster.FindCard(ctx, cardID)
		if err != nil {
			return nil, err
		}
		if card.GetInitialRarity() < minimumRarity {
			return nil, status.Errorf(codes.FailedPrecondition, "master-derived gacha card %q is below guaranteed rarity %d", cardID, minimumRarity)
		}
		seen[cardID] = struct{}{}
		pool = append(pool, &pmaster.RewardResource{
			ResourceType: penum.ResourceType_ResourceType_Card,
			ResourceId:   cardID,
			Amount:       1,
		})
	}
	if len(pool) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "master-derived guaranteed-rarity gacha pool is empty")
	}
	return pool, nil
}

func guaranteedGachaBatchPool(
	ctx context.Context,
	pool []*pmaster.RewardResource,
	buttonDrawCount int32,
	batchSize int32,
	minimumRarity int32,
) ([]*pmaster.RewardResource, map[string]int32, error) {
	if buttonDrawCount != batchSize || batchSize <= 0 || minimumRarity <= 0 {
		return nil, nil, nil
	}
	cardRarities := make(map[string]int32, len(pool))
	guaranteed := make([]*pmaster.RewardResource, 0)
	for _, reward := range pool {
		if reward.GetResourceType() != penum.ResourceType_ResourceType_Card {
			return nil, nil, nil
		}
		card, err := opmaster.FindCard(ctx, reward.GetResourceId())
		if err != nil {
			return nil, nil, err
		}
		cardRarities[card.GetId()] = card.GetInitialRarity()
		if card.GetInitialRarity() >= minimumRarity {
			guaranteed = append(guaranteed, reward)
		}
	}
	if len(guaranteed) == 0 {
		return nil, nil, status.Errorf(codes.FailedPrecondition, "gacha has no cards at guaranteed rarity %d", minimumRarity)
	}
	return guaranteed, cardRarities, nil
}

func grantGachaCardRewards(
	ctx context.Context,
	suid string,
	masterGacha *pmaster.Gacha,
	drawn *pmaster.RewardResource,
	overflowGifts *viber.GachaOverflowGiftBatch,
	updated *papi.UpdatedData,
) ([]*ptransaction.Reward, []*ptransaction.RewardResult, error) {
	if drawn.GetResourceType() != penum.ResourceType_ResourceType_Card {
		return nil, nil, nil
	}
	rewards := make([]*ptransaction.Reward, 0)
	results := make([]*ptransaction.RewardResult, 0)
	for _, definition := range masterGacha.GetCardRewards() {
		if definition.GetCardId() != drawn.GetResourceId() || definition.GetRewardId() == "" {
			continue
		}
		resources, err := resolveGachaMasterReward(ctx, definition.GetRewardId())
		if err != nil {
			return nil, nil, err
		}
		for _, resource := range resources {
			if resource.GetAmount() <= 0 {
				return nil, nil, status.Error(codes.FailedPrecondition, "gacha card reward must have a positive amount")
			}
			resourceID := resource.GetResourceId()
			if resource.GetResourceType() == penum.ResourceType_ResourceType_CardRarityExpLimitChange {
				// Gacha CardRewards type 25 is entitlement/limit metadata. Captured
				// responses expose it in CardRewards without changing rarity EXP.
				if resourceID == "" {
					resourceID = drawn.GetResourceId()
				}
			} else {
				granted, err := viber.GrantGachaDrawReward(ctx, suid, resource, overflowGifts, updated)
				if err != nil {
					return nil, nil, err
				}
				results = append(results, granted...)
			}
			rewards = append(rewards, &ptransaction.Reward{
				ResourceType: resource.GetResourceType(),
				ResourceId:   resourceID,
				Amount:       int64(resource.GetAmount()),
			})
		}
	}
	return rewards, results, nil
}

func resolveGachaMasterReward(ctx context.Context, rewardID string) ([]*pmaster.RewardResource, error) {
	resources, err := viber.ParseAutoReward(rewardID)
	if err != nil {
		return nil, err
	}
	if resources != nil {
		return resources, nil
	}
	reward, err := opmaster.FindReward(ctx, rewardID)
	if err != nil {
		return nil, err
	}
	return reward.GetResources(), nil
}

func fallbackCardPool(ctx context.Context, now int64, minimumRarity int32) ([]*pmaster.RewardResource, error) {
	cards, err := opmaster.FindAllCards(ctx)
	if err != nil {
		return nil, err
	}
	pool := make([]*pmaster.RewardResource, 0, len(cards))
	for _, card := range cards {
		if card.GetInitialRarity() < minimumRarity || card.GetReleaseDate() > now {
			continue
		}
		pool = append(pool, &pmaster.RewardResource{
			ResourceType: penum.ResourceType_ResourceType_Card,
			ResourceId:   card.GetId(),
			Amount:       1,
		})
	}
	return pool, nil
}

func transactionRewardsToMaster(rewards []*ptransaction.Reward) []*pmaster.RewardResource {
	result := make([]*pmaster.RewardResource, 0, len(rewards))
	for _, reward := range rewards {
		if reward.GetAmount() <= 0 || reward.GetAmount() > math.MaxInt32 {
			continue
		}
		result = append(result, &pmaster.RewardResource{
			ResourceType: reward.GetResourceType(),
			ResourceId:   reward.GetResourceId(),
			Amount:       int32(reward.GetAmount()),
		})
	}
	return result
}

func masterRewardsToTransaction(rewards []*pmaster.RewardResource) []*ptransaction.Reward {
	result := make([]*ptransaction.Reward, 0, len(rewards))
	for _, reward := range rewards {
		result = append(result, &ptransaction.Reward{
			ResourceType: reward.GetResourceType(),
			ResourceId:   reward.GetResourceId(),
			Amount:       int64(reward.GetAmount()),
		})
	}
	return result
}

func selectGachaReward(
	ctx context.Context,
	pool []*pmaster.RewardResource,
	masterGacha *pmaster.Gacha,
	selectedCardIDs []string,
) (*pmaster.RewardResource, error) {
	weights, err := gachaRewardWeights(ctx, masterGacha, selectedCardIDs, pool)
	if err != nil {
		return nil, err
	}
	var total int64
	for _, weight := range weights {
		total += weight
	}
	if total <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "gacha reward weights are empty")
	}
	value, err := rand.Int(rand.Reader, big.NewInt(total))
	if err != nil {
		return nil, err
	}
	target := value.Int64()
	for i, weight := range weights {
		if target < weight {
			return proto.Clone(pool[i]).(*pmaster.RewardResource), nil
		}
		target -= weight
	}
	return nil, status.Error(codes.Internal, "failed to select gacha reward")
}

func gachaRewardWeights(
	ctx context.Context,
	masterGacha *pmaster.Gacha,
	selectedCardIDs []string,
	pool []*pmaster.RewardResource,
) ([]int64, error) {
	if len(pool) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "gacha reward pool is empty")
	}
	if masterGacha.GetGachaType() == penum.GachaType_GachaType_Premium {
		weighted, err := viber.BuildPremiumGachaPool(ctx, masterGacha)
		if err != nil {
			return nil, err
		}
		if len(weighted) != len(pool) {
			return nil, status.Error(codes.FailedPrecondition, "premium gacha pool does not match its probability catalog")
		}
		weights := make([]int64, len(pool))
		for i, entry := range weighted {
			if !sameGachaReward(entry.Reward, pool[i]) {
				return nil, status.Error(codes.FailedPrecondition, "premium gacha pool order does not match its probability catalog")
			}
			weights[i] = entry.Probability
		}
		return weights, nil
	}
	if masterGacha.GetGachaType() == penum.GachaType_GachaType_Item ||
		masterGacha.GetGachaType() == penum.GachaType_GachaType_ShowcaseToy {
		return uniformGachaWeights(len(pool)), nil
	}

	rules, err := viber.LoadGachaRules()
	if err != nil {
		return nil, err
	}
	cards, err := opmaster.FindAllCards(ctx)
	if err != nil {
		return nil, err
	}
	cardRarity := make(map[string]int32, len(cards))
	for _, card := range cards {
		cardRarity[card.GetId()] = card.GetInitialRarity()
	}
	buckets := make(map[int32][]int)
	for i, reward := range pool {
		if reward.GetResourceType() != penum.ResourceType_ResourceType_Card {
			return nil, status.Error(codes.FailedPrecondition, "standard gacha pool contains a non-card reward")
		}
		rarity := cardRarity[reward.GetResourceId()]
		if rules.StandardCardRarityProbabilities[rarity] <= 0 {
			return nil, status.Errorf(codes.FailedPrecondition, "card %q has unsupported gacha rarity %d", reward.GetResourceId(), rarity)
		}
		buckets[rarity] = append(buckets[rarity], i)
	}
	rarities := make([]int, 0, len(buckets))
	var configuredTotal int64
	for rarity := range buckets {
		rarities = append(rarities, int(rarity))
		configuredTotal += rules.StandardCardRarityProbabilities[rarity]
	}
	sort.Ints(rarities)
	weights := make([]int64, len(pool))
	remainingTotal := int64(gachaProbabilityTotal)
	selected := viber.StringSet(selectedCardIDs)
	pickups := viber.StringSet(masterGacha.GetPickupCardIds())
	for rarityIndex, rarityValue := range rarities {
		rarity := int32(rarityValue)
		bucketTotal := int64(gachaProbabilityTotal) * rules.StandardCardRarityProbabilities[rarity] / configuredTotal
		if rarityIndex == len(rarities)-1 {
			bucketTotal = remainingTotal
		}
		remainingTotal -= bucketTotal
		indices := buckets[rarity]
		ratioUpIndices := make([]int, 0)
		regularIndices := make([]int, 0, len(indices))
		for _, poolIndex := range indices {
			resourceID := pool[poolIndex].GetResourceId()
			_, pickup := pickups[resourceID]
			_, chosen := selected[resourceID]
			if pickup || chosen {
				ratioUpIndices = append(ratioUpIndices, poolIndex)
			} else {
				regularIndices = append(regularIndices, poolIndex)
			}
		}
		ratioUpTotal := rules.RatioUpCardProbability * int64(len(ratioUpIndices))
		if len(regularIndices) == 0 || ratioUpTotal >= bucketTotal {
			assignGachaWeightEvenly(weights, ratioUpIndices, bucketTotal)
			continue
		}
		for _, poolIndex := range ratioUpIndices {
			weights[poolIndex] = rules.RatioUpCardProbability
		}
		assignGachaWeightEvenly(weights, regularIndices, bucketTotal-ratioUpTotal)
	}
	return weights, nil
}

func assignGachaWeightEvenly(weights []int64, indices []int, total int64) {
	if len(indices) == 0 {
		return
	}
	remaining := total
	for i, poolIndex := range indices {
		weight := total / int64(len(indices))
		if i == len(indices)-1 {
			weight = remaining
		}
		remaining -= weight
		weights[poolIndex] = weight
	}
}

func uniformGachaWeights(count int) []int64 {
	weights := make([]int64, count)
	remaining := int64(gachaProbabilityTotal)
	for i := range weights {
		weight := int64(gachaProbabilityTotal) / int64(count)
		if i == count-1 {
			weight = remaining
		}
		remaining -= weight
		weights[i] = weight
	}
	return weights
}

func sameGachaReward(left, right *pmaster.RewardResource) bool {
	return left.GetResourceType() == right.GetResourceType() &&
		left.GetResourceId() == right.GetResourceId() &&
		left.GetAmount() == right.GetAmount()
}

func grantGachaButtonBonus(
	ctx context.Context,
	suid string,
	button *pmaster.GachaButton,
	previousTotal int32,
	execCount int32,
	expiredTime int64,
	overflowGifts *viber.GachaOverflowGiftBatch,
	updated *papi.UpdatedData,
) ([]*ptransaction.Reward, []*ptransaction.RewardResult, error) {
	if button.GetBonusRewardId() == "" || button.GetBonusCount() <= 0 {
		return []*ptransaction.Reward{}, []*ptransaction.RewardResult{}, nil
	}
	before := previousTotal / button.GetBonusCount()
	after := (previousTotal + execCount) / button.GetBonusCount()
	occurrences := after - before
	if occurrences <= 0 {
		return []*ptransaction.Reward{}, []*ptransaction.RewardResult{}, nil
	}
	resources, err := resolveGachaMasterReward(ctx, button.GetBonusRewardId())
	if err != nil {
		return nil, nil, err
	}
	rewards := make([]*ptransaction.Reward, 0, len(resources))
	results := make([]*ptransaction.RewardResult, 0, len(resources))
	for _, resource := range resources {
		if int64(resource.GetAmount())*int64(occurrences) > math.MaxInt32 {
			return nil, nil, status.Error(codes.FailedPrecondition, "gacha bonus amount overflows")
		}
		scaled := proto.Clone(resource).(*pmaster.RewardResource)
		scaled.Amount *= occurrences
		rewards = append(rewards, &ptransaction.Reward{
			ResourceType: scaled.GetResourceType(),
			ResourceId:   scaled.GetResourceId(),
			Amount:       int64(scaled.GetAmount()),
		})
		var granted []*ptransaction.RewardResult
		if scaled.GetResourceType() == penum.ResourceType_ResourceType_Item {
			granted, err = viber.GrantGachaExchangeResource(
				ctx, suid, scaled.GetResourceType(), scaled.GetResourceId(), int64(scaled.GetAmount()), expiredTime, updated,
			)
		} else {
			granted, err = viber.GrantGachaDrawReward(ctx, suid, scaled, overflowGifts, updated)
		}
		if err != nil {
			return nil, nil, err
		}
		results = append(results, granted...)
	}
	return rewards, results, nil
}

func appendGachaHistories(
	ctx context.Context,
	suid string,
	gachaType penum.GachaType,
	entries []*papi.GachaHistory,
) error {
	history, err := opuser.FindUserGachaHistory(ctx, suid, gachaType)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		history = &umodel.UserGachaHistory{SUID: suid, GachaType: gachaType}
	}
	for i := 0; i < len(entries); i++ {
		history.Histories = append([]*papi.GachaHistory{entries[i]}, history.Histories...)
	}
	if len(history.Histories) > gachaHistoryLimit {
		history.Histories = history.Histories[:gachaHistoryLimit]
	}
	return opuser.UpsertUserGachaHistory(ctx, history)
}

func calculateGachaProbabilities(
	ctx context.Context,
	masterGacha *pmaster.Gacha,
	state *papi.GachaInfo,
	pool []*pmaster.RewardResource,
) ([]*papi.ProbabilityInfo, []*papi.RarityProbabilityInfo, error) {
	if len(pool) == 0 {
		return []*papi.ProbabilityInfo{}, []*papi.RarityProbabilityInfo{}, nil
	}
	selected := viber.StringSet(state.GetSelectedCardIds())
	pickups := viber.StringSet(masterGacha.GetPickupCardIds())
	weights, err := gachaRewardWeights(ctx, masterGacha, state.GetSelectedCardIds(), pool)
	if err != nil {
		return nil, nil, err
	}
	var totalWeight int64
	for _, weight := range weights {
		totalWeight += weight
	}

	probabilities := make([]*papi.ProbabilityInfo, 0, len(pool))
	remaining := int64(gachaProbabilityTotal)
	isPremium := masterGacha.GetGachaType() == penum.GachaType_GachaType_Premium
	for i, reward := range pool {
		probability := weights[i]
		if !isPremium {
			probability = int64(gachaProbabilityTotal) * weights[i] / totalWeight
			if i == len(pool)-1 {
				probability = remaining
			}
			remaining -= probability
		}
		if probability <= 0 || probability > math.MaxInt32 {
			return nil, nil, status.Error(codes.FailedPrecondition, "gacha reward probability is invalid")
		}
		_, isPickup := pickups[reward.GetResourceId()]
		_, isSelected := selected[reward.GetResourceId()]
		probabilities = append(probabilities, &papi.ProbabilityInfo{
			ResourceType: reward.GetResourceType(),
			ResourceId:   reward.GetResourceId(),
			Amount:       int64(reward.GetAmount()),
			Probability:  int32(probability),
			IsRatioUp:    isPickup || isSelected,
		})
	}

	if isPremium {
		return probabilities, []*papi.RarityProbabilityInfo{}, nil
	}
	cards, err := opmaster.FindAllCards(ctx)
	if err != nil {
		return nil, nil, err
	}
	cardRarity := make(map[string]int32, len(cards))
	for _, card := range cards {
		cardRarity[card.GetId()] = card.GetInitialRarity()
	}
	byRarity := make(map[int32][]*papi.ProbabilityInfo)
	for _, probability := range probabilities {
		if probability.GetResourceType() != penum.ResourceType_ResourceType_Card {
			continue
		}
		rarity := cardRarity[probability.GetResourceId()]
		if rarity > 0 {
			byRarity[rarity] = append(byRarity[rarity], probability)
		}
	}
	rarities := make([]int, 0, len(byRarity))
	for rarity := range byRarity {
		rarities = append(rarities, int(rarity))
	}
	sort.Sort(sort.Reverse(sort.IntSlice(rarities)))
	rarityProbabilities := make([]*papi.RarityProbabilityInfo, 0, len(rarities))
	for _, rarity := range rarities {
		entries := byRarity[int32(rarity)]
		var total int32
		for _, entry := range entries {
			total += entry.GetProbability()
		}
		pattern := penum.GachaRewardPatternType_GachaRewardPatternType_NotSelectCard
		if masterGacha.GetSelectPickupAmount() > 0 {
			pattern = penum.GachaRewardPatternType_GachaRewardPatternType_SelectCard
		} else if masterGacha.GetGachaType() == penum.GachaType_GachaType_Premium {
			pattern = penum.GachaRewardPatternType_GachaRewardPatternType_Premium
		}
		individual := int32(0)
		if len(entries) > 0 {
			individual = entries[0].GetProbability()
		}
		rarityProbabilities = append(rarityProbabilities, &papi.RarityProbabilityInfo{
			Rarity:                 int32(rarity),
			Probability:            total,
			ProbabilityInfos:       entries,
			GachaRewardPatternType: pattern,
			IndividualProbability:  individual,
		})
	}
	return probabilities, rarityProbabilities, nil
}
