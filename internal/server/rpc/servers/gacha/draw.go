package gacha

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
		state, err := currentGachaInfo(ctx, suid, catalog, masterGacha, now)
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
		overflowGiftBatch := newGachaOverflowGiftBatch(now)
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
		var normalTiers []normalGachaTier
		var guaranteedPool []*pmaster.RewardResource
		var cardRarities map[string]int32
		if isNormalCardGacha(masterGacha) {
			// Prepare once: normal draws first choose the rarity, then a card
			// uniformly within it, without reloading master data for every draw.
			normalTiers, err = buildNormalGachaTiers(ctx, pool)
			cardRarities = make(map[string]int32, len(pool))
			for _, tier := range normalTiers {
				for _, reward := range tier.rewards {
					cardRarities[reward.GetResourceId()] = tier.rarity
				}
				if button.GetDrawCount() == drawRules.GuaranteedCardBatchSize &&
					tier.rarity >= drawRules.GuaranteedCardMinimumRarity {
					guaranteedPool = append(guaranteedPool, tier.rewards...)
				}
			}
			if err == nil && button.GetDrawCount() == drawRules.GuaranteedCardBatchSize && len(guaranteedPool) == 0 {
				return nil, status.Errorf(codes.FailedPrecondition, "gacha has no cards at guaranteed rarity %d", drawRules.GuaranteedCardMinimumRarity)
			}
		} else {
			guaranteedPool, cardRarities, err = guaranteedGachaBatchPool(
				ctx, pool, button.GetDrawCount(), drawRules.GuaranteedCardBatchSize, drawRules.GuaranteedCardMinimumRarity,
			)
		}
		if err != nil {
			return nil, err
		}
		batchHasGuaranteedRarity := false
		for drawIndex := int64(0); drawIndex < drawCount; drawIndex++ {
			var selected *pmaster.RewardResource
			guaranteedDraw := len(guaranteedPool) > 0 &&
				(drawIndex+1)%int64(drawRules.GuaranteedCardBatchSize) == 0 && !batchHasGuaranteedRarity
			if premiumRemaining == 1 {
				selected = proto.Clone(pool[0]).(*pmaster.RewardResource)
			} else if len(normalTiers) > 0 {
				minimumRarity := int32(0)
				if guaranteedDraw {
					minimumRarity = drawRules.GuaranteedCardMinimumRarity
				}
				selected, err = selectNormalGachaReward(normalTiers, minimumRarity)
			} else if guaranteedDraw {
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
			results, err := grantGachaDrawReward(ctx, suid, selected, overflowGiftBatch, updated)
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
		exchangeResults, err := grantGachaExchangeResource(
			ctx,
			suid,
			masterGacha.GetExchangeResourceType(),
			masterGacha.GetExchangeResourceId(),
			drawCount,
			gachaTermEnd(ctx, masterGacha.GetViewConditionId()),
			updated,
		)
		if err != nil {
			return nil, err
		}
		drawResultList = append(drawResultList, exchangeResults...)

		bonusExpiration, err := gachaButtonBonusExpiration(ctx, masterGacha.GetViewConditionId())
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
		if err := flushGachaOverflowGifts(ctx, overflowGiftBatch, suid, updated); err != nil {
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
		resultList, err = aggregateGachaRewardResults(resultList)
		if err != nil {
			return nil, err
		}
		drawResultList, err = aggregateGachaRewardResults(drawResultList)
		if err != nil {
			return nil, err
		}
		canonicalizeGachaUpdatedData(updated)

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

type gachaRewardResultKey struct {
	resourceType penum.ResourceType
	resourceID   string
}

type gachaItemKey struct {
	itemID      string
	expiredTime int64
}

// aggregateGachaRewardResults coalesces every repeated drawn/granted resource,
// including cards. Nested duplicate rewards and additional gifts are coalesced
// by the same resource identity so repeated cards retain one canonical result.
func aggregateGachaRewardResults(results []*ptransaction.RewardResult) ([]*ptransaction.RewardResult, error) {
	return gachaResponseAggregateGachaRewardResults(results)
}

func gachaResponseAggregateGachaRewardResults(results []*ptransaction.RewardResult) ([]*ptransaction.RewardResult, error) {
	aggregated := make([]*ptransaction.RewardResult, 0, len(results))
	byResource := make(map[gachaRewardResultKey]*ptransaction.RewardResult, len(results))
	for _, result := range results {
		if result == nil {
			return nil, status.Error(codes.Internal, "gacha reward result is missing")
		}
		candidate := proto.Clone(result).(*ptransaction.RewardResult)
		var err error
		candidate.DuplicateRewardResults, err = gachaResponseAggregateGachaRewardResults(candidate.GetDuplicateRewardResults())
		if err != nil {
			return nil, err
		}
		candidate.AdditionalGifts, err = aggregateGachaRewards(candidate.GetAdditionalGifts())
		if err != nil {
			return nil, err
		}
		key := gachaRewardResultKey{resourceType: result.GetResourceType(), resourceID: result.GetResourceId()}
		current := byResource[key]
		if current == nil {
			current = candidate
			byResource[key] = current
			aggregated = append(aggregated, current)
			continue
		}
		if candidate.GetAmountI64() > 0 && current.GetAmountI64() > math.MaxInt64-candidate.GetAmountI64() {
			return nil, status.Error(codes.FailedPrecondition, "aggregated gacha reward amount overflows")
		}
		currentIsAdditive := isAdditiveGachaRewardResult(current)
		candidateIsAdditive := isAdditiveGachaRewardResult(candidate)
		duplicateResults, err := gachaResponseAggregateGachaRewardResults(append(current.GetDuplicateRewardResults(), candidate.GetDuplicateRewardResults()...))
		if err != nil {
			return nil, err
		}
		additionalGifts, err := aggregateGachaRewards(append(current.GetAdditionalGifts(), candidate.GetAdditionalGifts()...))
		if err != nil {
			return nil, err
		}
		current.AmountI64 += candidate.GetAmountI64()
		if currentIsAdditive && candidateIsAdditive && current.GetBeforeAmount() <= math.MaxInt64-current.GetAmountI64() {
			// Official grouped results report an identity-local post-image even
			// when grants for other resources were interleaved in the draw.
			current.AfterAmount = current.GetBeforeAmount() + current.GetAmountI64()
		} else {
			current.AfterAmount = candidate.GetAfterAmount()
		}
		current.IsNew = current.GetIsNew() || candidate.GetIsNew()
		current.IsTruncate = current.GetIsTruncate() || candidate.GetIsTruncate()
		current.IsGift = current.GetIsGift() || candidate.GetIsGift()
		current.DuplicateRewardResults = duplicateResults
		current.AdditionalGifts = additionalGifts
		if candidate.GetIncreasedAfterEvolutionLevel() > current.GetIncreasedAfterEvolutionLevel() {
			current.IncreasedAfterEvolutionLevel = candidate.GetIncreasedAfterEvolutionLevel()
		}
	}
	return aggregated, nil
}

func isAdditiveGachaRewardResult(result *ptransaction.RewardResult) bool {
	return result.GetResourceType() != penum.ResourceType_ResourceType_Card &&
		!result.GetIsTruncate() && result.GetAmountI64() >= 0 && result.GetBeforeAmount() >= 0 &&
		result.GetBeforeAmount() <= math.MaxInt64-result.GetAmountI64() &&
		result.GetAfterAmount() == result.GetBeforeAmount()+result.GetAmountI64()
}

func aggregateGachaRewards(rewards []*ptransaction.Reward) ([]*ptransaction.Reward, error) {
	aggregated := make([]*ptransaction.Reward, 0, len(rewards))
	byResource := make(map[gachaRewardResultKey]*ptransaction.Reward, len(rewards))
	for _, reward := range rewards {
		if reward == nil {
			return nil, status.Error(codes.Internal, "additional gacha reward is missing")
		}
		key := gachaRewardResultKey{resourceType: reward.GetResourceType(), resourceID: reward.GetResourceId()}
		current := byResource[key]
		if current == nil {
			current = proto.Clone(reward).(*ptransaction.Reward)
			byResource[key] = current
			aggregated = append(aggregated, current)
			continue
		}
		if reward.GetAmount() > 0 && current.GetAmount() > math.MaxInt64-reward.GetAmount() {
			return nil, status.Error(codes.FailedPrecondition, "aggregated additional gacha reward amount overflows")
		}
		current.Amount += reward.GetAmount()
	}
	return aggregated, nil
}

func canonicalizeGachaUpdatedData(updated *papi.UpdatedData) {
	if updated == nil {
		return
	}
	updated.Items = dedupeGachaUpdates(updated.Items, func(value *ptransaction.UserItem) gachaItemKey {
		return gachaItemKey{itemID: value.GetItemId(), expiredTime: value.GetExpiredTime()}
	})
	updated.Cards = dedupeGachaUpdates(updated.Cards, func(value *ptransaction.UserCard) string { return value.GetCardId() })
	updated.Characters = dedupeGachaUpdates(updated.Characters, func(value *ptransaction.UserCharacter) string { return value.GetCharacterId() })
	updated.Costumes = dedupeGachaUpdates(updated.Costumes, func(value *ptransaction.UserCostume) string { return value.GetCostumeId() })
	updated.Accessories = dedupeGachaUpdates(updated.Accessories, func(value *ptransaction.UserAccessory) string { return value.GetAccessoryId() })
	updated.Stories = dedupeGachaUpdates(updated.Stories, func(value *ptransaction.UserStory) string { return value.GetStoryId() })
	updated.Points = dedupeGachaUpdates(updated.Points, func(value *ptransaction.UserPoint) penum.PointType { return value.GetPointType() })
	updated.Emblems = dedupeGachaUpdates(updated.Emblems, func(value *ptransaction.UserEmblem) string { return value.GetEmblemId() })
	updated.Missions = dedupeGachaUpdates(updated.Missions, func(value *ptransaction.UserMission) string { return value.GetMissionId() })
	updated.GachaButtons = dedupeGachaUpdates(updated.GachaButtons, func(value *ptransaction.UserGachaButton) string { return value.GetGachaButtonId() })
	updated.Notifications = dedupeGachaUpdates(updated.Notifications, func(value *ptransaction.UserNotification) penum.NotificationType { return value.GetNotificationType() })
	updated.Decorations = dedupeGachaUpdates(updated.Decorations, func(value *ptransaction.UserDecoration) string { return value.GetDecorationId() })
	updated.PhotoRecipes = dedupeGachaUpdates(updated.PhotoRecipes, func(value *ptransaction.UserPhotoRecipe) string { return value.GetPhotoRecipeId() })
	updated.Hairs = dedupeGachaUpdates(updated.Hairs, func(value *ptransaction.UserHair) string { return value.GetHairId() })
	updated.ShowcaseToys = dedupeGachaUpdates(updated.ShowcaseToys, func(value *ptransaction.UserShowcaseToy) string { return value.GetShowcaseToyId() })
	updated.PhotoPoses = dedupeGachaUpdates(updated.PhotoPoses, func(value *ptransaction.UserPhotoPose) string { return value.GetPhotoPoseId() })
}

func dedupeGachaUpdates[T any, K comparable](values []*T, key func(*T) K) []*T {
	result := make([]*T, 0, len(values))
	indices := make(map[K]int, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		identity := key(value)
		if index, ok := indices[identity]; ok {
			result[index] = value
			continue
		}
		indices[identity] = len(result)
		result = append(result, value)
	}
	return result
}

func newGachaOverflowGiftBatch(postedTime int64) *viber.GachaOverflowGiftBatch {
	return &viber.GachaOverflowGiftBatch{PostedTime: postedTime, ByCardID: make(map[string]int64)}
}

func flushGachaOverflowGifts(ctx context.Context, b *viber.GachaOverflowGiftBatch, suid string, updated *papi.UpdatedData) error {
	if b == nil || len(b.ByCardID) == 0 {
		return nil
	}
	cardIDs := make([]string, 0, len(b.ByCardID))
	for cardID := range b.ByCardID {
		cardIDs = append(cardIDs, cardID)
	}
	sort.Strings(cardIDs)
	for _, cardID := range cardIDs {
		if err := viber.PersistGachaCardLimitBreakGift(ctx, suid, b.ByCardID[cardID], b.PostedTime); err != nil {
			return err
		}
	}
	return viber.EnsureGachaGiftNotification(ctx, suid, updated)
}

func grantGachaExchangeResource(
	ctx context.Context,
	suid string,
	resourceType penum.ResourceType,
	resourceID string,
	amount int64,
	expiredTime int64,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	if resourceType == penum.ResourceType_ResourceType_Unknown && resourceID == "" {
		return nil, nil
	}
	if resourceType == penum.ResourceType_ResourceType_Unknown || resourceID == "" {
		return nil, status.Error(codes.FailedPrecondition, "gacha exchange reward is incomplete")
	}
	if amount <= 0 || amount > math.MaxInt32 {
		return nil, status.Error(codes.FailedPrecondition, "gacha exchange reward amount is invalid")
	}
	if resourceType == penum.ResourceType_ResourceType_Item {
		result, err := viber.GrantExchangeItemWithExpiration(ctx, suid, resourceID, amount, expiredTime, updated)
		if err != nil {
			return nil, err
		}
		return []*ptransaction.RewardResult{result}, nil
	}
	return viber.GrantExchangeRewards(ctx, suid, &pmaster.RewardResource{
		ResourceType: resourceType,
		ResourceId:   resourceID,
		Amount:       int32(amount),
	}, updated)
}

func grantGachaDrawReward(
	ctx context.Context,
	suid string,
	reward *pmaster.RewardResource,
	overflowGifts *viber.GachaOverflowGiftBatch,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	if overflowGifts == nil {
		return nil, status.Error(codes.Internal, "gacha draw overflow gift batch is required")
	}
	return viber.GrantGachaReward(ctx, suid, reward, overflowGifts, updated)
}

func gachaButtonBonusExpiration(ctx context.Context, conditionID string) (int64, error) {
	end := gachaTermEnd(ctx, conditionID)
	if end <= 0 {
		return 0, nil
	}
	rules, err := viber.LoadGachaRules()
	if err != nil {
		return 0, err
	}
	grace := rules.ButtonBonusExpirationGraceHours * time.Hour.Milliseconds()
	if grace <= 0 || end > math.MaxInt64-grace {
		return 0, status.Error(codes.FailedPrecondition, "gacha button bonus expiration is invalid")
	}
	return end + grace, nil
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
		switch resource.GetResourceType() {
		case penum.ResourceType_ResourceType_Stone, penum.ResourceType_ResourceType_PaidStone:
			balanceResults, err := viber.ConsumeExchangeBalance(ctx, suid, resource.GetResourceType(), amount, updated)
			if err != nil {
				return nil, err
			}
			results = append(results, balanceResults...)
		default:
			return nil, status.Errorf(codes.Unimplemented, "gacha consumption resource type %s is unsupported", resource.GetResourceType())
		}
	}
	return results, nil
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
				granted, err := grantGachaDrawReward(ctx, suid, resource, overflowGifts, updated)
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

// Select rarity before the card so integer probability-display rounding never
// biases a card's draw chance. Guaranteed slots use the same eligible-tier odds.
func selectNormalGachaReward(tiers []normalGachaTier, minimumRarity int32) (*pmaster.RewardResource, error) {
	var total int64
	for _, tier := range tiers {
		if tier.rarity < minimumRarity {
			continue
		}
		if tier.weight <= 0 || len(tier.rewards) == 0 || tier.weight > math.MaxInt64-total {
			return nil, status.Error(codes.FailedPrecondition, "normal gacha rarity weights are invalid")
		}
		total += tier.weight
	}
	if total == 0 {
		return nil, status.Error(codes.FailedPrecondition, "normal gacha has no eligible rarity")
	}
	value, err := rand.Int(rand.Reader, big.NewInt(total))
	if err != nil {
		return nil, err
	}
	target := value.Int64()
	for _, tier := range tiers {
		if tier.rarity < minimumRarity {
			continue
		}
		if target < tier.weight {
			index, err := rand.Int(rand.Reader, big.NewInt(int64(len(tier.rewards))))
			if err != nil {
				return nil, err
			}
			return proto.Clone(tier.rewards[index.Int64()]).(*pmaster.RewardResource), nil
		}
		target -= tier.weight
	}
	return nil, status.Error(codes.Internal, "failed to select normal gacha reward")
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
			granted, err = grantGachaExchangeResource(
				ctx, suid, scaled.GetResourceType(), scaled.GetResourceId(), int64(scaled.GetAmount()), expiredTime, updated,
			)
		} else {
			granted, err = grantGachaDrawReward(ctx, suid, scaled, overflowGifts, updated)
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
