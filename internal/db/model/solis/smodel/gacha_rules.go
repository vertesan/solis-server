package smodel

import (
	"fmt"

	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

type GachaRules struct {
	StandardCardRarityProbabilities map[int32]int64    `yaml:"standardCardRarityProbabilities" json:"standardCardRarityProbabilities" bson:"standardCardRarityProbabilities"`
	RatioUpCardProbability          int64              `yaml:"ratioUpCardProbability" json:"ratioUpCardProbability" bson:"ratioUpCardProbability"`
	GuaranteedCardBatchSize         int32              `yaml:"guaranteedCardBatchSize" json:"guaranteedCardBatchSize" bson:"guaranteedCardBatchSize"`
	GuaranteedCardMinimumRarity     int32              `yaml:"guaranteedCardMinimumRarity" json:"guaranteedCardMinimumRarity" bson:"guaranteedCardMinimumRarity"`
	PremiumGuaranteeDrawCount       int32              `yaml:"premiumGuaranteeDrawCount" json:"premiumGuaranteeDrawCount" bson:"premiumGuaranteeDrawCount"`
	PremiumMasterRewardProbability  int64              `yaml:"premiumMasterRewardProbability" json:"premiumMasterRewardProbability" bson:"premiumMasterRewardProbability"`
	ButtonBonusExpirationGraceHours int64              `yaml:"buttonBonusExpirationGraceHours" json:"buttonBonusExpirationGraceHours" bson:"buttonBonusExpirationGraceHours"`
	PremiumItems                    []GachaCatalogItem `yaml:"premiumItems" json:"premiumItems" bson:"premiumItems"`
	ContinuousGachas                []GachaContinuous  `yaml:"continuousGachas" json:"continuousGachas" bson:"continuousGachas"`
	NoticeAliases                   []GachaNoticeAlias `yaml:"noticeAliases" json:"noticeAliases" bson:"noticeAliases"`
	GuaranteedRarityGachas          []GachaRarityPool  `yaml:"guaranteedRarityGachas" json:"guaranteedRarityGachas" bson:"guaranteedRarityGachas"`
	ItemGachas                      []GachaCatalogPool `yaml:"itemGachas" json:"itemGachas" bson:"itemGachas"`
	ShowcaseToyGachas               []GachaCatalogPool `yaml:"showcaseToyGachas" json:"showcaseToyGachas" bson:"showcaseToyGachas"`
}

type GachaNoticeAlias struct {
	GachaID  string `yaml:"gachaId" json:"gachaId" bson:"gachaId"`
	NoticeID string `yaml:"noticeId" json:"noticeId" bson:"noticeId"`
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
	ID                   string `yaml:"id" json:"id" bson:"id"`
	MinimumRarity        int32  `yaml:"minimumRarity" json:"minimumRarity" bson:"minimumRarity"`
	UseMasterCardRewards bool   `yaml:"useMasterCardRewards" json:"useMasterCardRewards" bson:"useMasterCardRewards"`
}

type GachaContinuous struct {
	ID               string `yaml:"id" json:"id" bson:"id"`
	ExternalRewardID string `yaml:"externalRewardId" json:"externalRewardId" bson:"externalRewardId"`
	RewardID         string `yaml:"rewardId" json:"rewardId" bson:"rewardId"`
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
	ItemID      string `yaml:"itemId" json:"itemId" bson:"itemId"`
	Amount      int32  `yaml:"amount" json:"amount" bson:"amount"`
	Probability int64  `yaml:"probability" json:"probability" bson:"probability"`
	Featured    bool   `yaml:"featured" json:"featured" bson:"featured"`
}

type GachaCatalogPool struct {
	ID      string               `yaml:"id" json:"id" bson:"id"`
	Rewards []GachaCatalogReward `yaml:"rewards" json:"rewards" bson:"rewards"`
}

type GachaCatalogReward struct {
	ResourceType int32  `yaml:"resourceType" json:"resourceType" bson:"resourceType"`
	ResourceID   string `yaml:"resourceId" json:"resourceId" bson:"resourceId"`
	Amount       int32  `yaml:"amount" json:"amount" bson:"amount"`
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

func (rules *GachaRules) Validate() error {
	if rules == nil {
		return fmt.Errorf("gacha rules are required")
	}
	if len(rules.StandardCardRarityProbabilities) == 0 || rules.RatioUpCardProbability <= 0 || rules.RatioUpCardProbability > 100000 ||
		rules.GuaranteedCardBatchSize <= 0 || rules.GuaranteedCardMinimumRarity <= 0 ||
		rules.PremiumGuaranteeDrawCount <= 0 || rules.PremiumMasterRewardProbability <= 0 || rules.PremiumMasterRewardProbability > 100000 ||
		rules.ButtonBonusExpirationGraceHours <= 0 || rules.ButtonBonusExpirationGraceHours > 24*365 {
		return fmt.Errorf("gacha catalog has invalid probability rules")
	}
	premiumTotal := rules.PremiumMasterRewardProbability
	for _, item := range rules.PremiumItems {
		if item.ItemID == "" || item.Amount <= 0 || item.Probability <= 0 || item.Probability > 100000-premiumTotal {
			return fmt.Errorf("gacha catalog has an invalid premium item")
		}
		premiumTotal += item.Probability
	}
	// Probability values are displayed as integer parts per 100,000. The
	// official premium catalog rounds each entry independently, so the visible
	// entries may undershoot 100,000 by at most one unit per configured item.
	premiumRoundingGap := int64(100000) - premiumTotal
	if premiumRoundingGap < 0 || premiumRoundingGap > int64(len(rules.PremiumItems)) {
		return fmt.Errorf("premium gacha probabilities total %d is outside the rounding tolerance", premiumTotal)
	}
	var standardTotal int64
	for rarity, probability := range rules.StandardCardRarityProbabilities {
		if rarity <= 0 || probability <= 0 || probability > 100000-standardTotal {
			return fmt.Errorf("gacha catalog has an invalid rarity probability")
		}
		standardTotal += probability
	}
	if standardTotal != 100000 {
		return fmt.Errorf("standard gacha probabilities total %d, want 100000", standardTotal)
	}
	for _, pool := range append(append([]GachaCatalogPool(nil), rules.ItemGachas...), rules.ShowcaseToyGachas...) {
		if pool.ID == "" || len(pool.Rewards) == 0 {
			return fmt.Errorf("gacha catalog has an invalid reward pool")
		}
		for _, reward := range pool.Rewards {
			if reward.ResourceType <= 0 || reward.ResourceID == "" || reward.Amount <= 0 {
				return fmt.Errorf("gacha catalog pool %q has an invalid reward", pool.ID)
			}
		}
	}
	for _, continuous := range rules.ContinuousGachas {
		if continuous.ID == "" || continuous.ExternalRewardID == "" || continuous.RewardID == "" {
			return fmt.Errorf("gacha catalog has an invalid continuous gacha")
		}
	}
	seenNoticeGacha := make(map[string]struct{}, len(rules.NoticeAliases))
	for _, alias := range rules.NoticeAliases {
		if alias.GachaID == "" || alias.NoticeID == "" {
			return fmt.Errorf("gacha catalog has an invalid notice alias")
		}
		if _, exists := seenNoticeGacha[alias.GachaID]; exists {
			return fmt.Errorf("gacha catalog repeats notice alias %q", alias.GachaID)
		}
		seenNoticeGacha[alias.GachaID] = struct{}{}
	}
	seenRarityGacha := make(map[string]struct{}, len(rules.GuaranteedRarityGachas))
	for _, pool := range rules.GuaranteedRarityGachas {
		if pool.ID == "" || pool.MinimumRarity <= 0 {
			return fmt.Errorf("gacha catalog has an invalid guaranteed-rarity pool")
		}
		if _, exists := seenRarityGacha[pool.ID]; exists {
			return fmt.Errorf("gacha catalog repeats guaranteed-rarity pool %q", pool.ID)
		}
		seenRarityGacha[pool.ID] = struct{}{}
	}
	return nil
}
