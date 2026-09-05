package viber

import (
	"embed"
	"fmt"

	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.yaml.in/yaml/v3"
)

// gachaCatalogFiles retains only the embedded file index. The raw YAML bytes
// read by LoadGachaRules are local and can be reclaimed after decoding.
//
//go:embed catalogdata/gacha.yaml
var gachaCatalogFiles embed.FS

type GachaRules struct {
	StandardCardRarityProbabilities map[int32]int64    `yaml:"standardCardRarityProbabilities"`
	RatioUpCardProbability          int64              `yaml:"ratioUpCardProbability"`
	GuaranteedCardBatchSize         int32              `yaml:"guaranteedCardBatchSize"`
	GuaranteedCardMinimumRarity     int32              `yaml:"guaranteedCardMinimumRarity"`
	PremiumGuaranteeDrawCount       int32              `yaml:"premiumGuaranteeDrawCount"`
	PremiumMasterRewardProbability  int64              `yaml:"premiumMasterRewardProbability"`
	ButtonBonusExpirationGraceHours int64              `yaml:"buttonBonusExpirationGraceHours"`
	PremiumItems                    []GachaCatalogItem `yaml:"premiumItems"`
	ContinuousGachas                []GachaContinuous  `yaml:"continuousGachas"`
	NoticeAliases                   []GachaNoticeAlias `yaml:"noticeAliases"`
	GuaranteedRarityGachas          []GachaRarityPool  `yaml:"guaranteedRarityGachas"`
	ItemGachas                      []GachaCatalogPool `yaml:"itemGachas"`
	ShowcaseToyGachas               []GachaCatalogPool `yaml:"showcaseToyGachas"`
}

type GachaNoticeAlias struct {
	GachaID  string `yaml:"gachaId"`
	NoticeID string `yaml:"noticeId"`
}

func (r *GachaRules) NoticeID(gachaID string) string {
	for _, alias := range r.NoticeAliases {
		if alias.GachaID == gachaID {
			return alias.NoticeID
		}
	}
	return ""
}

type GachaRarityPool struct {
	ID                   string `yaml:"id"`
	MinimumRarity        int32  `yaml:"minimumRarity"`
	UseMasterCardRewards bool   `yaml:"useMasterCardRewards"`
}

type GachaContinuous struct {
	ID               string `yaml:"id"`
	ExternalRewardID string `yaml:"externalRewardId"`
	RewardID         string `yaml:"rewardId"`
}

func (r *GachaRules) Continuous(id string) *GachaContinuous {
	for i := range r.ContinuousGachas {
		if r.ContinuousGachas[i].ID == id {
			return &r.ContinuousGachas[i]
		}
	}
	return nil
}

type GachaCatalogItem struct {
	ItemID      string `yaml:"itemId"`
	Amount      int32  `yaml:"amount"`
	Probability int64  `yaml:"probability"`
	Featured    bool   `yaml:"featured"`
}

type GachaCatalogPool struct {
	ID      string               `yaml:"id"`
	Rewards []GachaCatalogReward `yaml:"rewards"`
}

type GachaCatalogReward struct {
	ResourceType int32  `yaml:"resourceType"`
	ResourceID   string `yaml:"resourceId"`
	Amount       int32  `yaml:"amount"`
}

func (r GachaCatalogReward) MasterReward() *pmaster.RewardResource {
	return &pmaster.RewardResource{
		ResourceType: penum.ResourceType(r.ResourceType),
		ResourceId:   r.ResourceID,
		Amount:       r.Amount,
	}
}

func (r *GachaRules) ItemPool(id string) []*pmaster.RewardResource {
	return r.catalogPool(r.ItemGachas, id)
}

func (r *GachaRules) ShowcaseToyPool(id string) []*pmaster.RewardResource {
	return r.catalogPool(r.ShowcaseToyGachas, id)
}

func (r *GachaRules) catalogPool(pools []GachaCatalogPool, id string) []*pmaster.RewardResource {
	for _, pool := range pools {
		if pool.ID != id {
			continue
		}
		result := make([]*pmaster.RewardResource, 0, len(pool.Rewards))
		for _, reward := range pool.Rewards {
			result = append(result, reward.MasterReward())
		}
		return result
	}
	return nil
}

func LoadGachaRules() (*GachaRules, error) {
	data, err := gachaCatalogFiles.ReadFile("catalogdata/gacha.yaml")
	if err != nil {
		return nil, fmt.Errorf("read gacha catalog: %w", err)
	}
	var rules GachaRules
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("decode gacha catalog: %w", err)
	}
	if len(rules.StandardCardRarityProbabilities) == 0 || rules.RatioUpCardProbability <= 0 ||
		rules.GuaranteedCardBatchSize <= 0 || rules.GuaranteedCardMinimumRarity <= 0 ||
		rules.PremiumGuaranteeDrawCount <= 0 || rules.PremiumMasterRewardProbability <= 0 ||
		rules.ButtonBonusExpirationGraceHours <= 0 || rules.ButtonBonusExpirationGraceHours > 24*365 {
		return nil, fmt.Errorf("gacha catalog has invalid probability rules")
	}
	premiumTotal := rules.PremiumMasterRewardProbability
	for _, item := range rules.PremiumItems {
		if item.ItemID == "" || item.Amount <= 0 || item.Probability <= 0 {
			return nil, fmt.Errorf("gacha catalog has an invalid premium item")
		}
		premiumTotal += item.Probability
	}
	// Probability values are displayed as integer parts per 100,000. The
	// official premium catalog rounds each entry independently, so the visible
	// entries may undershoot 100,000 by at most one unit per configured item.
	premiumRoundingGap := int64(100000) - premiumTotal
	if premiumRoundingGap < 0 || premiumRoundingGap > int64(len(rules.PremiumItems)) {
		return nil, fmt.Errorf("premium gacha probabilities total %d is outside the rounding tolerance", premiumTotal)
	}
	var standardTotal int64
	for rarity, probability := range rules.StandardCardRarityProbabilities {
		if rarity <= 0 || probability <= 0 {
			return nil, fmt.Errorf("gacha catalog has an invalid rarity probability")
		}
		standardTotal += probability
	}
	if standardTotal != 100000 {
		return nil, fmt.Errorf("standard gacha probabilities total %d, want 100000", standardTotal)
	}
	for _, pool := range append(append([]GachaCatalogPool(nil), rules.ItemGachas...), rules.ShowcaseToyGachas...) {
		if pool.ID == "" || len(pool.Rewards) == 0 {
			return nil, fmt.Errorf("gacha catalog has an invalid reward pool")
		}
		for _, reward := range pool.Rewards {
			if reward.ResourceType <= 0 || reward.ResourceID == "" || reward.Amount <= 0 {
				return nil, fmt.Errorf("gacha catalog pool %q has an invalid reward", pool.ID)
			}
		}
	}
	for _, continuous := range rules.ContinuousGachas {
		if continuous.ID == "" || continuous.ExternalRewardID == "" || continuous.RewardID == "" {
			return nil, fmt.Errorf("gacha catalog has an invalid continuous gacha")
		}
	}
	seenNoticeGacha := make(map[string]struct{}, len(rules.NoticeAliases))
	for _, alias := range rules.NoticeAliases {
		if alias.GachaID == "" || alias.NoticeID == "" {
			return nil, fmt.Errorf("gacha catalog has an invalid notice alias")
		}
		if _, exists := seenNoticeGacha[alias.GachaID]; exists {
			return nil, fmt.Errorf("gacha catalog repeats notice alias %q", alias.GachaID)
		}
		seenNoticeGacha[alias.GachaID] = struct{}{}
	}
	seenRarityGacha := make(map[string]struct{}, len(rules.GuaranteedRarityGachas))
	for _, pool := range rules.GuaranteedRarityGachas {
		if pool.ID == "" || pool.MinimumRarity <= 0 {
			return nil, fmt.Errorf("gacha catalog has an invalid guaranteed-rarity pool")
		}
		if _, exists := seenRarityGacha[pool.ID]; exists {
			return nil, fmt.Errorf("gacha catalog repeats guaranteed-rarity pool %q", pool.ID)
		}
		seenRarityGacha[pool.ID] = struct{}{}
	}
	return &rules, nil
}
