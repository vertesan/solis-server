package market

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.yaml.in/yaml/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// marketCatalogFiles retains only the embedded file index. The raw bytes read
// from it are local to the one-time decoder and can be reclaimed afterwards.
//
//go:embed catalogdata/market.yaml
var marketCatalogFiles embed.FS

var (
	marketCatalogOnce sync.Once
	marketCatalog     *MarketCatalog
	marketCatalogErr  error
)

type MarketCatalog struct {
	PaidResetLimit             int32        `yaml:"paidResetLimit"`
	RequiredResetStoneAmount   int32        `yaml:"requiredResetStoneAmount"`
	RegularNumberOffset        int32        `yaml:"regularNumberOffset"`
	DiscountNumberOffset       int32        `yaml:"discountNumberOffset"`
	RegularUnlockConditionID   string       `yaml:"regularUnlockConditionId"`
	DiscountUnlockConditionID  string       `yaml:"discountUnlockConditionId"`
	DiscountDayConditionID     string       `yaml:"discountDayConditionId"`
	RegularDiscountRatePermils []int32      `yaml:"regularDiscountRatePermils"`
	DiscountDayRatePermil      int32        `yaml:"discountDayRatePermil"`
	Tiers                      []MarketTier `yaml:"tiers"`
}

type MarketTier struct {
	MaxManagerLevel int32        `yaml:"maxManagerLevel"`
	Slots           []MarketSlot `yaml:"slots"`
}

type MarketSlot struct {
	Number            int32               `yaml:"number"`
	Variants          []MarketSlotVariant `yaml:"variants"`
	MarketSlotVariant `yaml:",inline"`
}

type MarketSlotVariant struct {
	ResourceType    string   `yaml:"resourceType"`
	BaseAmount      int32    `yaml:"baseAmount"`
	RewardKind      string   `yaml:"rewardKind"`
	RewardIDs       []string `yaml:"rewardIds"`
	RewardAmount    int64    `yaml:"rewardAmount"`
	AccessoryRarity int32    `yaml:"accessoryRarity"`
}

func loadMarketCatalog() (*MarketCatalog, error) {
	marketCatalogOnce.Do(func() {
		data, err := marketCatalogFiles.ReadFile("catalogdata/market.yaml")
		if err != nil {
			marketCatalogErr = fmt.Errorf("read market catalog: %w", err)
			return
		}
		var catalog MarketCatalog
		if err := yaml.Unmarshal(data, &catalog); err != nil {
			marketCatalogErr = fmt.Errorf("decode market catalog: %w", err)
			return
		}
		marketCatalog = &catalog
	})
	return marketCatalog, marketCatalogErr
}

// jSTHalfDayStart returns the most recent 00:00 or 12:00 JST boundary.
func jSTHalfDayStart(now time.Time) time.Time {
	local := now.In(viber.JstLocation)
	hour := 0
	if local.Hour() >= 12 {
		hour = 12
	}
	return time.Date(local.Year(), local.Month(), local.Day(), hour, 0, 0, 0, viber.JstLocation)
}

// isJSTMarketDiscountDay reports the recurring market sale dates used after
// the finite dated conditions shipped in the client master data expire.
func isJSTMarketDiscountDay(now time.Time) bool {
	switch now.In(viber.JstLocation).Day() {
	case 10, 20, 30:
		return true
	default:
		return false
	}
}

var (
	errMarketItemSoldOut    = errors.New("market item is sold out")
	errMarketFreeResetUsed  = errors.New("market free reset is unavailable")
	errMarketPaidResetLimit = errors.New("market paid reset limit reached")
)

func findAndRefreshMarketState(
	ctx context.Context,
	suid string,
	now time.Time,
	catalog *MarketCatalog,
) (*umodel.UserMarketState, error) {
	state, err := opuser.FindUserMarketState(ctx, suid)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		state = &umodel.UserMarketState{SUID: suid}
	}
	changed := false
	boundary := jSTHalfDayStart(now)
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

func generateMarketItems(ctx context.Context, suid string, catalog *MarketCatalog, now time.Time) ([]*papi.MarketItem, error) {
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
	discountDay = discountDay || isJSTMarketDiscountDay(now)
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

func chooseMarketSlotVariant(slot MarketSlot) (MarketSlotVariant, error) {
	if len(slot.Variants) == 0 {
		return slot.MarketSlotVariant, nil
	}
	index, err := viber.CryptoRandomIndex(len(slot.Variants))
	if err != nil {
		return MarketSlotVariant{}, err
	}
	return slot.Variants[index], nil
}

func marketTierForLevel(catalog *MarketCatalog, level int32) *MarketTier {
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

func marketRewardID(ctx context.Context, slot MarketSlotVariant, accessoriesByRarity map[int32][]string) (string, error) {
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

func marketFreeResetRemainCount(state *umodel.UserMarketState, now time.Time) int32 {
	if state.FreeResetBoundaryTime == jSTHalfDayStart(now).UnixMilli() {
		return 0
	}
	return 1
}

func marketPaidResetRemainCount(state *umodel.UserMarketState, catalog *MarketCatalog) int32 {
	return max(0, catalog.PaidResetLimit-state.PaidResetCount)
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
