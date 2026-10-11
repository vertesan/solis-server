package moshikoi

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"slices"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.yaml.in/yaml/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//go:embed catalogdata/*.yaml
var catalogFiles embed.FS

// EventConfig contains server-owned data absent from the master database.
// Levels, costs, and episode assets are always obtained from master data.
type EventConfig struct {
	LoveID                 string          `yaml:"loveId"`
	Name                   string          `yaml:"name"`
	Description            string          `yaml:"description"`
	StartTime              int64           `yaml:"startTime"`
	UnlockConditionID      string          `yaml:"unlockConditionId"`
	BannerAssetID          string          `yaml:"bannerAssetId"`
	CharacterID            string          `yaml:"characterId"`
	CostumeID              string          `yaml:"costumeId"`
	CharacterAssetID       string          `yaml:"characterAssetId"`
	Color                  string          `yaml:"color"`
	StoryReadItemID        string          `yaml:"storyReadItemId"`
	PresentItemID          string          `yaml:"presentItemId"`
	PromoteItemID          string          `yaml:"promoteItemId"`
	PresentFavorability    int64           `yaml:"presentFavorability"`
	LessonStepSeconds      int64           `yaml:"lessonStepSeconds"`
	LessonMaxHours         int64           `yaml:"lessonMaxHours"`
	PromoteHours           int64           `yaml:"promoteHours"`
	PromoteMinutes         int64           `yaml:"-"`
	EventMissionIDs        []string        `yaml:"eventMissionIds"`
	LimitedEventMissionIDs []string        `yaml:"limitedEventMissionIds"`
	EpisodeUnlockLevels    map[int32]int32 `yaml:"episodeUnlockLevels"`
	HintUnlockLevels       map[int32]int32 `yaml:"hintUnlockLevels"`
}

type Catalog struct {
	LoveID       string
	Config       *EventConfig
	Levels       []*pmaster.LoveLevel
	Episodes     []*pmaster.LoveStoryEpisode
	TitleAssetID string
}

func SupportedLoveIDs() []string {
	return []string{"love-23-0514", "love-23-1114", "love-24-0517", "love-24-1114", "love-26-0313"}
}

func LoadConfig(loveID string) (*EventConfig, error) {
	if !slices.Contains(SupportedLoveIDs(), loveID) {
		return nil, status.Error(codes.NotFound, "love event is not available")
	}
	raw, err := catalogFiles.ReadFile("catalogdata/" + loveID + ".yaml")
	if err != nil {
		return nil, err
	}
	var cfg EventConfig
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err = decoder.Decode(&cfg); err != nil {
		return nil, err
	}
	if cfg.LoveID != loveID || cfg.Name == "" || cfg.CharacterID == "" || cfg.StoryReadItemID == "" || cfg.PresentItemID == "" || cfg.PromoteItemID == "" || cfg.StartTime <= 0 || cfg.LessonMaxHours <= 0 || cfg.LessonMaxHours > 24 {
		return nil, fmt.Errorf("invalid catalog for love event %s", loveID)
	}
	return &cfg, nil
}

func LoadCatalog(ctx context.Context, loveID string) (*Catalog, error) {
	cfg, err := LoadConfig(loveID)
	if err != nil {
		return nil, err
	}
	levels, err := opmaster.FindLoveLevels(ctx, loveID)
	if err != nil {
		return nil, err
	}
	episodes, err := opmaster.FindLoveStoryEpisodes(ctx, loveID)
	if err != nil {
		return nil, err
	}
	if len(levels) == 0 || len(episodes) == 0 {
		return nil, fmt.Errorf("missing master data for love event %s", loveID)
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return nil, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return nil, err
	}
	cfg.LessonStepSeconds = int64(setting.GetSecondsPerLoveLessonStep())
	present, err := opmaster.FindItem(ctx, cfg.PresentItemID)
	if err != nil {
		return nil, err
	}
	boost, err := opmaster.FindItem(ctx, cfg.PromoteItemID)
	if err != nil {
		return nil, err
	}
	if present.GetType() != penum.ItemType_ItemType_LoveHiddenFavorabilityObtain || boost.GetType() != penum.ItemType_ItemType_LoveLessonPromote || present.GetValue() <= 0 || boost.GetValue() <= 0 || cfg.LessonStepSeconds <= 0 {
		return nil, fmt.Errorf("invalid item/lesson master data for love event %s", loveID)
	}
	cfg.PresentFavorability = int64(present.GetValue())
	cfg.PromoteMinutes = int64(boost.GetValue())
	cfg.PromoteHours = cfg.PromoteMinutes / 60
	sort.Slice(levels, func(i, j int) bool { return levels[i].GetLevel() < levels[j].GetLevel() })
	sort.Slice(episodes, func(i, j int) bool { return episodes[i].GetId() < episodes[j].GetId() })
	for i, level := range levels {
		if level.GetLevel() <= 0 || level.GetRequiredExp() < 0 || level.GetLessonStepRewardExp() < 0 || level.GetLessonStepRewardStoryReadItemAmount() < 0 || i > 0 && levels[i-1].GetRequiredExp() >= level.GetRequiredExp() {
			return nil, fmt.Errorf("invalid love level master data for %s", loveID)
		}
	}
	return &Catalog{LoveID: loveID, Config: cfg, Levels: levels, Episodes: episodes, TitleAssetID: setting.GetLoveTitleAssetId()}, nil
}

func (c *Catalog) Level(exp int64) int32 { return c.LevelData(exp).GetLevel() }
func (c *Catalog) LevelData(exp int64) *pmaster.LoveLevel {
	current := c.Levels[0]
	for _, level := range c.Levels {
		if level.GetRequiredExp() > exp {
			break
		}
		current = level
	}
	return current
}
func (c *Catalog) Episode(id string) *pmaster.LoveStoryEpisode {
	for _, ep := range c.Episodes {
		if ep.GetId() == id {
			return ep
		}
	}
	return nil
}
func (c *Catalog) MainCharacterInfo() *papi.LoveMainCharacterInfo {
	return &papi.LoveMainCharacterInfo{CharacterId: c.Config.CharacterID, CostumeId: c.Config.CostumeID, AssetId: c.Config.CharacterAssetID, Color: c.Config.Color}
}
