package birthday

import (
	"context"
	"embed"
	"fmt"
	"slices"
	"strings"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.yaml.in/yaml/v3"
)

//go:embed catalogdata/birthday.yaml
var catalogFiles embed.FS

type Catalog struct {
	BannerAssetID string                   `yaml:"bannerAssetId"`
	Order         int32                    `yaml:"order"`
	DefaultText   string                   `yaml:"defaultText"`
	Texts         map[string]string        `yaml:"texts"`
	Reminder      *papi.BirthdayRemindInfo `yaml:"reminder"`
	Entries       map[string]*Entry        `yaml:"-"`
	CharacterIDs  []string                 `yaml:"-"`
}

type Entry struct {
	CharacterID  string
	DecorationID string
	Info         *papi.BirthdayInfo
}

func LoadCatalog(ctx context.Context) (*Catalog, error) {
	raw, err := catalogFiles.ReadFile("catalogdata/birthday.yaml")
	if err != nil {
		return nil, err
	}
	// Generated protobuf structs have JSON, not YAML field tags.
	var settings struct {
		BannerAssetID string            `yaml:"bannerAssetId"`
		Order         int32             `yaml:"order"`
		DefaultText   string            `yaml:"defaultText"`
		Texts         map[string]string `yaml:"texts"`
		Reminder      struct {
			AssetID     string `yaml:"assetId"`
			Description string `yaml:"description"`
			LinkTitle   string `yaml:"linkTitle"`
			LinkType    int32  `yaml:"linkType"`
		} `yaml:"reminder"`
	}
	if err := yaml.Unmarshal(raw, &settings); err != nil {
		return nil, err
	}
	catalog := &Catalog{BannerAssetID: settings.BannerAssetID, Order: settings.Order,
		DefaultText: settings.DefaultText, Texts: settings.Texts, Entries: make(map[string]*Entry)}
	catalog.Reminder = &papi.BirthdayRemindInfo{AssetId: settings.Reminder.AssetID,
		Description: settings.Reminder.Description, LinkTitle: settings.Reminder.LinkTitle}
	catalog.Reminder.LinkType = penum.LinkType(settings.Reminder.LinkType)
	characters, err := opmaster.FindAllCharacters(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*pmaster.Character, len(characters))
	for _, character := range characters {
		byID[character.GetId()] = character
	}
	decorations, err := opmaster.FindAllDecorations(ctx)
	if err != nil {
		return nil, err
	}
	for _, decoration := range decorations {
		code, ok := strings.CutPrefix(decoration.GetId(), "deco-user-birthday-")
		if !ok || !strings.HasSuffix(code, "-01") {
			continue
		}
		code = strings.TrimSuffix(code, "-01")
		character := byID["char-"+code]
		if character == nil {
			continue
		}
		costume := "cos-" + code + "-casl-00"
		if !slices.Contains(character.GetCostumeIds(), costume) {
			costume = character.GetDefaultCostumeId()
		}
		text := catalog.Texts[character.GetId()]
		if text == "" {
			text = catalog.DefaultText
		}
		entry := &Entry{CharacterID: character.GetId(), DecorationID: decoration.GetId(),
			Info: &papi.BirthdayInfo{BirthdayAdvInfo: &papi.BirthdayAdvInfo{
				BirthdayId: "birthday-01-" + code + "-01", AdvAssetId: "userhbd_01_" + code,
			}, Text: text, CostumeId: costume}}
		catalog.Entries[entry.Info.GetBirthdayAdvInfo().GetBirthdayId()] = entry
		catalog.CharacterIDs = append(catalog.CharacterIDs, character.GetId())
	}
	if len(catalog.Entries) < 3 {
		return nil, fmt.Errorf("birthday catalog requires at least three supported characters")
	}
	slices.Sort(catalog.CharacterIDs)
	return catalog, nil
}
