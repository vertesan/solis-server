package viber

import (
	"bytes"
	"embed"
	"fmt"
	"sync"

	"go.yaml.in/yaml/v3"
)

type fanEventBalanceCatalog struct {
	Profiles                    []FanEventBalanceProfile `yaml:"profiles"`
	CoreFanConversionMillionths map[string]int32         `yaml:"coreFanConversionMillionths"`
	Reward                      FanEventRewardCatalog    `yaml:"reward"`
	RewardProfiles              []FanEventRewardProfile  `yaml:"rewardProfiles"`
}

type FanEventBalanceProfile struct {
	ID                             string           `yaml:"id"`
	Level                          int32            `yaml:"level"`
	ActivityPointCoefficientPermil int32            `yaml:"activityPointCoefficientPermil"`
	RankAThresholdPoint            int64            `yaml:"rankAThresholdPoint"`
	CharacterActivityExp           int64            `yaml:"characterActivityExp"`
	CoreFanConversionMillionths    map[string]int32 `yaml:"coreFanConversionMillionths"`
}

type FanEventRewardCatalog struct {
	RecipeDropChancePermyriads int32            `yaml:"recipeDropChancePermyriads"`
	RankCoefficientsPermyriads map[string]int32 `yaml:"rankCoefficientsPermyriads"`
	ManagerExp                 map[string]int64 `yaml:"managerExp"`
}

type FanEventRewardProfile struct {
	ID            string                           `yaml:"id"`
	Level         int32                            `yaml:"level"`
	DynamicByRank map[string]FanEventDynamicReward `yaml:"dynamicByRank"`
}

type FanEventDynamicReward struct {
	CardEnhance int64 `yaml:"cardEnhance"`
	CardSkill   int64 `yaml:"cardSkill"`
}

//go:embed catalogdata/fan_event_profiles.yaml
var fanEventBalanceFiles embed.FS

var (
	loadFanEventBalanceOnce sync.Once
	loadedFanEventBalance   *fanEventBalanceCatalog
	loadFanEventBalanceErr  error
)

func FindFanEventBalanceProfile(id string, level int32) (*FanEventBalanceProfile, error) {
	loadFanEventBalanceOnce.Do(func() {
		data, err := fanEventBalanceFiles.ReadFile("catalogdata/fan_event_profiles.yaml")
		if err != nil {
			loadFanEventBalanceErr = fmt.Errorf("read fan event balance catalog: %w", err)
			return
		}
		catalog := new(fanEventBalanceCatalog)
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(catalog); err != nil {
			loadFanEventBalanceErr = fmt.Errorf("decode fan event balance catalog: %w", err)
			return
		}
		for _, profile := range catalog.Profiles {
			if profile.ID == "" || profile.Level <= 0 || profile.ActivityPointCoefficientPermil <= 0 ||
				profile.RankAThresholdPoint <= 0 || profile.CharacterActivityExp <= 0 {
				loadFanEventBalanceErr = fmt.Errorf("fan event balance catalog contains an invalid profile")
				return
			}
			for rank, coefficient := range profile.CoreFanConversionMillionths {
				if !validFanEventRankKey(rank) || coefficient <= 0 {
					loadFanEventBalanceErr = fmt.Errorf("fan event profile core-fan coefficient %q is invalid", rank)
					return
				}
			}
		}
		for _, key := range fanEventRankKeys {
			if catalog.CoreFanConversionMillionths[key] <= 0 {
				loadFanEventBalanceErr = fmt.Errorf("fan event core-fan rank %q is invalid", key)
				return
			}
		}
		if catalog.Reward.RecipeDropChancePermyriads < 0 || catalog.Reward.RecipeDropChancePermyriads > 10000 {
			loadFanEventBalanceErr = fmt.Errorf("fan event reward drop chance is invalid")
			return
		}
		for _, key := range fanEventRankKeys {
			if catalog.Reward.RankCoefficientsPermyriads[key] <= 0 || catalog.Reward.ManagerExp[key] <= 0 {
				loadFanEventBalanceErr = fmt.Errorf("fan event reward rank %q is invalid", key)
				return
			}
		}
		for _, profile := range catalog.RewardProfiles {
			if profile.ID == "" || profile.Level <= 0 {
				loadFanEventBalanceErr = fmt.Errorf("fan event dynamic reward profile is invalid")
				return
			}
			for _, reward := range profile.DynamicByRank {
				if reward.CardEnhance <= 0 || reward.CardSkill <= 0 {
					loadFanEventBalanceErr = fmt.Errorf("fan event dynamic reward amount is invalid")
					return
				}
			}
		}
		loadedFanEventBalance = catalog
	})
	if loadFanEventBalanceErr != nil {
		return nil, loadFanEventBalanceErr
	}
	for index := range loadedFanEventBalance.Profiles {
		profile := &loadedFanEventBalance.Profiles[index]
		if profile.ID == id && profile.Level == level {
			return profile, nil
		}
	}
	return nil, nil
}

var fanEventRankKeys = []string{"D", "C", "B", "A", "S", "Splus"}

func validFanEventRankKey(candidate string) bool {
	for _, key := range fanEventRankKeys {
		if candidate == key {
			return true
		}
	}
	return false
}

func FanEventCoreFanConversionMillionths(profile *FanEventBalanceProfile, rankKey string) (int32, error) {
	if _, err := FindFanEventBalanceProfile("", 0); err != nil {
		return 0, err
	}
	if profile != nil {
		if coefficient := profile.CoreFanConversionMillionths[rankKey]; coefficient > 0 {
			return coefficient, nil
		}
	}
	coefficient := loadedFanEventBalance.CoreFanConversionMillionths[rankKey]
	if coefficient <= 0 {
		return 0, fmt.Errorf("fan event core-fan rank %q is unavailable", rankKey)
	}
	return coefficient, nil
}

func LoadFanEventRewardCatalog() (*FanEventRewardCatalog, error) {
	if _, err := FindFanEventBalanceProfile("", 0); err != nil {
		return nil, err
	}
	return &loadedFanEventBalance.Reward, nil
}

func FindFanEventRewardProfile(id string, level int32) (*FanEventRewardProfile, error) {
	if _, err := FindFanEventBalanceProfile("", 0); err != nil {
		return nil, err
	}
	for index := range loadedFanEventBalance.RewardProfiles {
		profile := &loadedFanEventBalance.RewardProfiles[index]
		if profile.ID == id && profile.Level == level {
			return profile, nil
		}
	}
	return nil, nil
}
