package user

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"sync"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/pmaster"

	"go.yaml.in/yaml/v3"
)

type godActivityCatalog struct {
	Lesson     godLessonCatalog      `yaml:"lesson"`
	FanEvents  []godFanEventCatalog  `yaml:"fanEvents"`
	Promotions []godPromotionCatalog `yaml:"promotions"`
	Refreshes  []godRefreshCatalog   `yaml:"refreshes"`
}

type godLessonCatalog struct {
	CurrentAreaID             string `yaml:"currentAreaId"`
	StepRewardGold            int32  `yaml:"stepRewardGold"`
	StepRewardManagerExp      int32  `yaml:"stepRewardManagerExp"`
	StepRewardCardEnhanceItem int32  `yaml:"stepRewardCardEnhanceItem"`
}

type godFanEventCatalog struct {
	ID                     string `yaml:"id"`
	Level                  int32  `yaml:"level"`
	ActivityPerformanceID  string `yaml:"activityPerformanceId"`
	RequiredStamina        int32  `yaml:"requiredStamina"`
	StepConsumptionStamina int32  `yaml:"stepConsumptionStamina"`
	FixedRewardID          string `yaml:"fixedRewardId"`
}

type godPromotionCatalog struct {
	ID                        string `yaml:"id"`
	Level                     int32  `yaml:"level"`
	ActivityPerformanceID     string `yaml:"activityPerformanceId"`
	RequiredStamina           int32  `yaml:"requiredStamina"`
	StepConsumptionStamina    int32  `yaml:"stepConsumptionStamina"`
	StepRewardGold            int32  `yaml:"stepRewardGold"`
	StepRewardCardEnhanceItem int32  `yaml:"stepRewardCardEnhanceItem"`
	MultiStepRewardID         string `yaml:"multiStepRewardId"`
	MaxFanAmount              int32  `yaml:"maxFanAmount"`
	SubGenre                  string `yaml:"subGenre"`
}

type godRefreshCatalog struct {
	ID              string                   `yaml:"id"`
	RequiredMinutes int32                    `yaml:"requiredMinutes"`
	Levels          []godRefreshLevelCatalog `yaml:"levels"`
}

type godRefreshLevelCatalog struct {
	Level                 int32  `yaml:"level"`
	RequiredGold          int32  `yaml:"requiredGold"`
	ActivityPerformanceID string `yaml:"activityPerformanceId"`
	RecoveryAmount        int32  `yaml:"recoveryAmount"`
	SubGenre              string `yaml:"subGenre"`
}

// godActivityCatalogFiles stores the embedded file index. ReadFile allocates
// the raw YAML locally during the one-time parse, so that buffer can be
// reclaimed after decoding.
//
//go:embed catalogdata/god_activity.yaml
var godActivityCatalogFiles embed.FS

var (
	loadGodActivityCatalogOnce sync.Once
	loadedGodActivityCatalog   *godActivityCatalog
	loadGodActivityCatalogErr  error
)

func loadGodActivityCatalog() (*godActivityCatalog, error) {
	loadGodActivityCatalogOnce.Do(func() {
		data, err := godActivityCatalogFiles.ReadFile("catalogdata/god_activity.yaml")
		if err != nil {
			loadGodActivityCatalogErr = fmt.Errorf("failed to read god activity catalog: %w", err)
			return
		}
		catalog := new(godActivityCatalog)
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(catalog); err != nil {
			loadGodActivityCatalogErr = fmt.Errorf("failed to decode god activity catalog: %w", err)
			return
		}
		loadedGodActivityCatalog = catalog
	})
	return loadedGodActivityCatalog, loadGodActivityCatalogErr
}

func validateGodActivityCatalog(
	ctx context.Context,
	catalog *godActivityCatalog,
	areas []*pmaster.Area,
	masterFanEvents []*pmaster.ActivityFanEvent,
	masterPromotions []*pmaster.ActivityPromotion,
	masterRefreshes []*pmaster.ActivityRefresh,
) error {
	if catalog == nil {
		return fmt.Errorf("catalog is empty")
	}
	mainArea, _ := furthestGodAreas(areas)
	if mainArea == nil {
		return fmt.Errorf("master data has no main area")
	}
	if catalog.Lesson.CurrentAreaID != mainArea.GetId() {
		return fmt.Errorf(
			"lesson area %q does not match master maximum %q",
			catalog.Lesson.CurrentAreaID,
			mainArea.GetId(),
		)
	}
	if catalog.Lesson.StepRewardGold <= 0 || catalog.Lesson.StepRewardManagerExp <= 0 ||
		catalog.Lesson.StepRewardCardEnhanceItem <= 0 {
		return fmt.Errorf("lesson reward amounts must be positive")
	}

	masterFansByID := make(map[string]*pmaster.ActivityFanEvent, len(masterFanEvents))
	for _, master := range masterFanEvents {
		masterFansByID[master.GetId()] = master
	}
	seenFans := make(map[string]struct{}, len(catalog.FanEvents))
	rewardIDs := make(map[string]struct{})
	for _, profile := range catalog.FanEvents {
		if _, duplicate := seenFans[profile.ID]; duplicate {
			return fmt.Errorf("fan event %q is duplicated", profile.ID)
		}
		seenFans[profile.ID] = struct{}{}
		master := masterFansByID[profile.ID]
		if master == nil {
			return fmt.Errorf("fan event %q is missing from master data", profile.ID)
		}
		maxLevel := maxFanEventLevel(master)
		if maxLevel == nil || profile.Level != maxLevel.GetLevel() {
			return fmt.Errorf("fan event %q level %d does not match master maximum", profile.ID, profile.Level)
		}
		if profile.ActivityPerformanceID == "" || profile.RequiredStamina <= 0 ||
			profile.StepConsumptionStamina <= 0 || profile.FixedRewardID == "" {
			return fmt.Errorf("fan event %q has an incomplete max-level profile", profile.ID)
		}
		rewardIDs[profile.FixedRewardID] = struct{}{}
	}
	if len(seenFans) != len(masterFansByID) {
		return fmt.Errorf("fan event catalog covers %d of %d master entries", len(seenFans), len(masterFansByID))
	}

	masterPromotionsByID := make(map[string]*pmaster.ActivityPromotion, len(masterPromotions))
	for _, master := range masterPromotions {
		masterPromotionsByID[master.GetId()] = master
	}
	seenPromotions := make(map[string]struct{}, len(catalog.Promotions))
	for _, profile := range catalog.Promotions {
		if _, duplicate := seenPromotions[profile.ID]; duplicate {
			return fmt.Errorf("promotion %q is duplicated", profile.ID)
		}
		seenPromotions[profile.ID] = struct{}{}
		master := masterPromotionsByID[profile.ID]
		if master == nil {
			return fmt.Errorf("promotion %q is missing from master data", profile.ID)
		}
		maxLevel := maxPromotionLevel(master)
		if maxLevel == nil || profile.Level != maxLevel.GetLevel() {
			return fmt.Errorf("promotion %q level %d does not match master maximum", profile.ID, profile.Level)
		}
		if profile.ActivityPerformanceID == "" || profile.RequiredStamina <= 0 ||
			profile.StepConsumptionStamina <= 0 || profile.StepRewardGold <= 0 ||
			profile.StepRewardCardEnhanceItem <= 0 || profile.MultiStepRewardID == "" ||
			profile.MaxFanAmount <= 0 || profile.SubGenre == "" {
			return fmt.Errorf("promotion %q has an incomplete max-level profile", profile.ID)
		}
		rewardIDs[profile.MultiStepRewardID] = struct{}{}
	}
	if len(seenPromotions) != len(masterPromotionsByID) {
		return fmt.Errorf("promotion catalog covers %d of %d master entries", len(seenPromotions), len(masterPromotionsByID))
	}

	masterRefreshesByID := make(map[string]*pmaster.ActivityRefresh, len(masterRefreshes))
	for _, master := range masterRefreshes {
		masterRefreshesByID[master.GetId()] = master
	}
	seenRefreshes := make(map[string]struct{}, len(catalog.Refreshes))
	for _, profile := range catalog.Refreshes {
		if _, duplicate := seenRefreshes[profile.ID]; duplicate {
			return fmt.Errorf("refresh %q is duplicated", profile.ID)
		}
		seenRefreshes[profile.ID] = struct{}{}
		master := masterRefreshesByID[profile.ID]
		if master == nil {
			return fmt.Errorf("refresh %q is missing from master data", profile.ID)
		}
		if profile.RequiredMinutes <= 0 {
			return fmt.Errorf("refresh %q has an invalid duration", profile.ID)
		}
		masterLevels := make(map[int32]struct{}, len(master.GetLevels()))
		for _, level := range master.GetLevels() {
			masterLevels[level.GetLevel()] = struct{}{}
		}
		seenLevels := make(map[int32]struct{}, len(profile.Levels))
		for _, level := range profile.Levels {
			if _, duplicate := seenLevels[level.Level]; duplicate {
				return fmt.Errorf("refresh %q level %d is duplicated", profile.ID, level.Level)
			}
			seenLevels[level.Level] = struct{}{}
			if _, exists := masterLevels[level.Level]; !exists {
				return fmt.Errorf("refresh %q level %d is missing from master data", profile.ID, level.Level)
			}
			if level.RequiredGold < 0 || level.ActivityPerformanceID == "" ||
				level.RecoveryAmount <= 0 || level.SubGenre == "" {
				return fmt.Errorf("refresh %q level %d has an incomplete profile", profile.ID, level.Level)
			}
		}
		if len(seenLevels) != len(masterLevels) {
			return fmt.Errorf(
				"refresh %q catalog covers %d of %d master levels",
				profile.ID,
				len(seenLevels),
				len(masterLevels),
			)
		}
	}
	if len(seenRefreshes) != len(masterRefreshesByID) {
		return fmt.Errorf("refresh catalog covers %d of %d master entries", len(seenRefreshes), len(masterRefreshesByID))
	}

	for rewardID := range rewardIDs {
		if _, err := opmaster.FindReward(ctx, rewardID); err != nil {
			return fmt.Errorf("activity reward %q is missing from master data: %w", rewardID, err)
		}
	}
	return nil
}

func maxFanEventLevel(master *pmaster.ActivityFanEvent) *pmaster.ActivityFanEventLevel {
	var result *pmaster.ActivityFanEventLevel
	for _, level := range master.GetLevels() {
		if result == nil || level.GetLevel() > result.GetLevel() {
			result = level
		}
	}
	return result
}

func maxPromotionLevel(master *pmaster.ActivityPromotion) *pmaster.ActivityPromotionLevel {
	var result *pmaster.ActivityPromotionLevel
	for _, level := range master.GetLevels() {
		if result == nil || level.GetLevel() > result.GetLevel() {
			result = level
		}
	}
	return result
}

func (catalog *godActivityCatalog) fanEvent(id string) *godFanEventCatalog {
	for index := range catalog.FanEvents {
		if catalog.FanEvents[index].ID == id {
			return &catalog.FanEvents[index]
		}
	}
	return nil
}

func (catalog *godActivityCatalog) promotion(id string) *godPromotionCatalog {
	for index := range catalog.Promotions {
		if catalog.Promotions[index].ID == id {
			return &catalog.Promotions[index]
		}
	}
	return nil
}

func (catalog *godActivityCatalog) refresh(id string) *godRefreshCatalog {
	for index := range catalog.Refreshes {
		if catalog.Refreshes[index].ID == id {
			return &catalog.Refreshes[index]
		}
	}
	return nil
}

func (catalog *godRefreshCatalog) level(number int32) *godRefreshLevelCatalog {
	for index := range catalog.Levels {
		if catalog.Levels[index].Level == number {
			return &catalog.Levels[index]
		}
	}
	return nil
}
