package viber

import (
	"context"
	"fmt"
	"strings"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type GachaMasterCatalog struct {
	Gachas    map[string]*pmaster.Gacha
	Buttons   map[string]*pmaster.GachaButton
	Exchanges map[string]*pmaster.GachaExchange
	Ordered   []*pmaster.Gacha
}

type WeightedGachaReward struct {
	Reward      *pmaster.RewardResource
	Probability int64
}

func LoadGachaMasterCatalog(ctx context.Context) (*GachaMasterCatalog, error) {
	gachas, err := opmaster.FindAllGachas(ctx)
	if err != nil {
		return nil, err
	}
	buttons, err := opmaster.FindAllGachaButtons(ctx)
	if err != nil {
		return nil, err
	}
	exchanges, err := opmaster.FindAllGachaExchanges(ctx)
	if err != nil {
		return nil, err
	}
	catalog := &GachaMasterCatalog{
		Gachas:    make(map[string]*pmaster.Gacha, len(gachas)),
		Buttons:   make(map[string]*pmaster.GachaButton, len(buttons)),
		Exchanges: make(map[string]*pmaster.GachaExchange, len(exchanges)),
		Ordered:   gachas,
	}
	for _, gacha := range gachas {
		catalog.Gachas[gacha.GetId()] = gacha
	}
	for _, button := range buttons {
		catalog.Buttons[button.GetId()] = button
	}
	for _, exchange := range exchanges {
		catalog.Exchanges[exchange.GetId()] = exchange
	}
	return catalog, nil
}

// ReconcileCurrentGachaInfos is the single current-rotation projection used
// by Gacha.List and Home.Enter. It preserves mutable counters/selections while
// deriving catalog identity, visibility and unlock state from master data.
func ReconcileCurrentGachaInfos(ctx context.Context, suid string, now int64) ([]*papi.GachaInfo, error) {
	catalog, err := LoadGachaMasterCatalog(ctx)
	if err != nil {
		return nil, err
	}
	rules, err := LoadGachaRules()
	if err != nil {
		return nil, err
	}
	states, err := opuser.FindAllUserGachaInfos(ctx, suid)
	if err != nil {
		return nil, err
	}
	stateByID := make(map[string]*papi.GachaInfo, len(states))
	for _, state := range states {
		stateByID[state.GetId()] = state
	}
	notices := loadGachaNotices(ctx)
	evaluator := NewCommerceConditionEvaluator(ctx, suid, now)
	conditionIDs := make([]string, 0, 2*len(catalog.Ordered))
	for _, gacha := range catalog.Ordered {
		conditionIDs = append(conditionIDs, gacha.GetViewConditionId(), gacha.GetUnlockConditionId())
	}
	if err := evaluator.PrimeConditionIDs(conditionIDs); err != nil {
		return nil, err
	}
	visible := make([]*papi.GachaInfo, 0, len(catalog.Ordered))
	for _, masterGacha := range catalog.Ordered {
		shown, err := evaluator.Satisfied(masterGacha.GetViewConditionId())
		if err != nil {
			return nil, err
		}
		if !shown {
			continue
		}
		info, err := RenderCurrentGachaInfo(
			ctx, evaluator, catalog, rules, masterGacha, stateByID[masterGacha.GetId()], notices, now,
		)
		if err != nil {
			return nil, err
		}
		// Home is opened frequently. Preserve the same reconciliation, including
		// daily resets and catalog changes, without rewriting identical rows.
		if !proto.Equal(stateByID[masterGacha.GetId()], info) {
			if err := opuser.UpsertUserGachaInfo(ctx, suid, info); err != nil {
				return nil, err
			}
		}
		visible = append(visible, info)
	}
	return visible, nil
}

func RenderCurrentGachaInfo(
	ctx context.Context,
	evaluator *CommerceConditionEvaluator,
	catalog *GachaMasterCatalog,
	rules *GachaRules,
	masterGacha *pmaster.Gacha,
	previous *papi.GachaInfo,
	notices map[string]*papi.NoticeInfo,
	now int64,
) (*papi.GachaInfo, error) {
	unlocked, err := evaluator.Satisfied(masterGacha.GetUnlockConditionId())
	if err != nil {
		return nil, err
	}
	info := &papi.GachaInfo{Id: masterGacha.GetId(), Unlocked: unlocked}
	if previous != nil {
		info.SelectedCardIds = append([]string(nil), previous.GetSelectedCardIds()...)
		if previous.GetContinuousInfo() != nil {
			info.ContinuousInfo = proto.Clone(previous.GetContinuousInfo()).(*papi.GachaContinuousInfo)
		}
		if previous.GetStampInfo() != nil {
			info.StampInfo = proto.Clone(previous.GetStampInfo()).(*papi.GachaStampInfo)
		}
	}
	if masterGacha.GetGachaType() == penum.GachaType_GachaType_Continuous && info.GetContinuousInfo() == nil {
		info.ContinuousInfo = &papi.GachaContinuousInfo{}
		if continuous := rules.Continuous(masterGacha.GetId()); continuous != nil {
			info.ContinuousInfo.ExternalRewardInfo = &papi.ExternalRewardInfo{
				ExternalRewardId: continuous.ExternalRewardID,
				RewardId:         continuous.RewardID,
			}
		}
	}

	previousButtons := make(map[string]*papi.GachaButtonInfo)
	if previous != nil {
		for _, button := range previous.GetButtons() {
			previousButtons[button.GetId()] = button
		}
	}
	for _, buttonID := range masterGacha.GetButtonIds() {
		if catalog.Buttons[buttonID] == nil {
			return nil, status.Errorf(codes.FailedPrecondition, "gacha button %q is missing from master data", buttonID)
		}
		button := &papi.GachaButtonInfo{Id: buttonID, DrawTime: now}
		if old := previousButtons[buttonID]; old != nil {
			button = proto.Clone(old).(*papi.GachaButtonInfo)
		}
		ResetGachaDailyCount(button, now)
		info.Buttons = append(info.Buttons, button)
	}

	previousExchanges := make(map[string]*papi.GachaExchangeInfo)
	if previous != nil {
		for _, exchange := range previous.GetExchanges() {
			previousExchanges[exchange.GetId()] = exchange
		}
	}
	for _, exchangeID := range masterGacha.GetExchangeIds() {
		if catalog.Exchanges[exchangeID] == nil {
			return nil, status.Errorf(codes.FailedPrecondition, "gacha exchange %q is missing from master data", exchangeID)
		}
		exchange := &papi.GachaExchangeInfo{Id: exchangeID, ExchangedTime: now}
		if old := previousExchanges[exchangeID]; old != nil {
			exchange = proto.Clone(old).(*papi.GachaExchangeInfo)
		}
		info.Exchanges = append(info.Exchanges, exchange)
	}

	noticeID := rules.NoticeID(masterGacha.GetId())
	if noticeID == "" {
		exactID := "info-" + masterGacha.GetId()
		if notices[exactID] != nil {
			noticeID = exactID
		}
	}
	if notice := notices[noticeID]; notice != nil {
		info.NoticeInfo = proto.Clone(notice).(*papi.NoticeInfo)
	} else if notices == nil && previous.GetNoticeInfo() != nil {
		// Draw/selection updates reuse the rendered state without loading the
		// notice catalog. Keep their existing announcement intact.
		info.NoticeInfo = proto.Clone(previous.GetNoticeInfo()).(*papi.NoticeInfo)
	} else {
		info.NoticeInfo = NewDummyNoticeInfo()
	}

	switch masterGacha.GetGachaType() {
	case penum.GachaType_GachaType_Item:
		pool := rules.ItemPool(masterGacha.GetId())
		if len(pool) == 0 && previous != nil {
			pool = transactionRewardsToMaster(previous.GetItemGachaRewards())
		}
		if err := validateGachaCatalogRewards(ctx, pool); err != nil {
			return nil, err
		}
		info.ItemGachaRewards = masterRewardsToTransaction(pool)
	case penum.GachaType_GachaType_ShowcaseToy:
		pool := rules.ShowcaseToyPool(masterGacha.GetId())
		if len(pool) == 0 && previous != nil {
			pool = transactionRewardsToMaster(previous.GetShowcaseToyGachaRewards())
		}
		if err := validateGachaCatalogRewards(ctx, pool); err != nil {
			return nil, err
		}
		info.ShowcaseToyGachaRewards = masterRewardsToTransaction(pool)
	case penum.GachaType_GachaType_Premium:
		premium, err := buildPremiumGachaInfo(ctx, catalog, rules, masterGacha, previous)
		if err != nil {
			return nil, err
		}
		info.PremiumInfo = premium
	}
	return info, nil
}

func BuildPremiumGachaPool(ctx context.Context, masterGacha *pmaster.Gacha) ([]WeightedGachaReward, error) {
	rules, err := LoadGachaRules()
	if err != nil {
		return nil, err
	}
	rewardID := premiumRewardID(masterGacha.GetId())
	reward, err := opmaster.FindReward(ctx, rewardID)
	if err != nil {
		return nil, fmt.Errorf("find premium reward %q: %w", rewardID, err)
	}
	if len(reward.GetResources()) != 1 {
		return nil, status.Errorf(codes.FailedPrecondition, "premium reward %q must contain exactly one resource", rewardID)
	}
	if err := validateGachaCatalogRewards(ctx, reward.GetResources()); err != nil {
		return nil, err
	}
	pool := []WeightedGachaReward{{
		Reward:      proto.Clone(reward.GetResources()[0]).(*pmaster.RewardResource),
		Probability: rules.PremiumMasterRewardProbability,
	}}
	for _, item := range rules.PremiumItems {
		resource := &pmaster.RewardResource{
			ResourceType: penum.ResourceType_ResourceType_Item,
			ResourceId:   item.ItemID,
			Amount:       item.Amount,
		}
		if err := validateGachaCatalogRewards(ctx, []*pmaster.RewardResource{resource}); err != nil {
			return nil, err
		}
		pool = append(pool, WeightedGachaReward{Reward: resource, Probability: item.Probability})
	}
	return pool, nil
}

func buildPremiumGachaInfo(
	ctx context.Context,
	catalog *GachaMasterCatalog,
	rules *GachaRules,
	masterGacha *pmaster.Gacha,
	previous *papi.GachaInfo,
) (*papi.GachaPremiumInfo, error) {
	receivable := true
	remaining := rules.PremiumGuaranteeDrawCount
	if previous != nil && previous.GetPremiumInfo() != nil {
		receivable = previous.GetPremiumInfo().GetIsFreeGachaTicketReceivable()
		if previous.GetPremiumInfo().GetFixedPremiumRewardRequiredDrawCount() > 0 {
			remaining = previous.GetPremiumInfo().GetFixedPremiumRewardRequiredDrawCount()
		}
	}
	premium := &papi.GachaPremiumInfo{
		FixedPremiumRewardRequiredDrawCount: remaining,
		PremiumRewardIds:                    []string{premiumRewardID(masterGacha.GetId())},
		IsFreeGachaTicketReceivable:         receivable,
	}
	pool, err := BuildPremiumGachaPool(ctx, masterGacha)
	if err != nil {
		return nil, err
	}
	for i, weighted := range pool {
		if i > 0 && !rules.PremiumItems[i-1].Featured {
			continue
		}
		premium.FeaturedRewards = append(premium.FeaturedRewards, &ptransaction.Reward{
			ResourceType: weighted.Reward.GetResourceType(),
			ResourceId:   weighted.Reward.GetResourceId(),
			Amount:       int64(weighted.Reward.GetAmount()),
		})
	}
	for _, buttonID := range masterGacha.GetButtonIds() {
		button := catalog.Buttons[buttonID]
		if button == nil || button.GetConsumptionId() == "" {
			continue
		}
		consumption, err := opmaster.FindConsumption(ctx, button.GetConsumptionId())
		if err != nil {
			continue
		}
		for _, resource := range consumption.GetResources() {
			if resource.GetResourceType() != penum.ResourceType_ResourceType_Item {
				continue
			}
			for _, itemID := range resource.GetResourceIds() {
				if strings.Contains(itemID, "free") {
					premium.FreeGachaTicketItemId = itemID
				}
			}
		}
	}
	return premium, nil
}

func premiumRewardID(gachaID string) string {
	return "rd-gacha-ratio-" + strings.TrimPrefix(gachaID, "gacha-") + "-01"
}

func validateGachaCatalogRewards(ctx context.Context, rewards []*pmaster.RewardResource) error {
	for _, reward := range rewards {
		if reward.GetResourceId() == "" || reward.GetAmount() <= 0 {
			return status.Error(codes.FailedPrecondition, "gacha catalog reward is invalid")
		}
		var err error
		switch reward.GetResourceType() {
		case penum.ResourceType_ResourceType_Card:
			_, err = opmaster.FindCard(ctx, reward.GetResourceId())
		case penum.ResourceType_ResourceType_Item:
			_, err = opmaster.FindItem(ctx, reward.GetResourceId())
		case penum.ResourceType_ResourceType_Accessory:
			_, err = opmaster.FindAccessory(ctx, reward.GetResourceId())
		case penum.ResourceType_ResourceType_ShowcaseToy:
			_, err = opmaster.FindShowcaseToy(ctx, reward.GetResourceId())
		default:
			return status.Errorf(codes.Unimplemented, "gacha catalog reward type %s is unsupported", reward.GetResourceType())
		}
		if err != nil {
			return fmt.Errorf("invalid gacha catalog resource %q: %w", reward.GetResourceId(), err)
		}
	}
	return nil
}

func loadGachaNotices(ctx context.Context) map[string]*papi.NoticeInfo {
	result := make(map[string]*papi.NoticeInfo)
	catalog, err := opsolis.FindNoticeCatalog(ctx)
	if err != nil {
		return result
	}
	for _, group := range [][]*papi.NoticeInfo{catalog.Notices, catalog.MalfunctionNotices, catalog.PrNotices} {
		for _, notice := range group {
			result[notice.GetId()] = notice
		}
	}
	return result
}

func ResetGachaDailyCount(button *papi.GachaButtonInfo, now int64) {
	if button.GetDrawTime() <= 0 || SameJSTDay(button.GetDrawTime(), now) {
		return
	}
	button.TodayCount = 0
}

func transactionRewardsToMaster(rewards []*ptransaction.Reward) []*pmaster.RewardResource {
	result := make([]*pmaster.RewardResource, 0, len(rewards))
	for _, reward := range rewards {
		if reward.GetAmount() <= 0 || reward.GetAmount() > int64(^uint32(0)>>1) {
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
