package viber

import (
	"embed"
	"fmt"
	"sync"

	"go.yaml.in/yaml/v3"
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

func LoadMarketCatalog() (*MarketCatalog, error) {
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
