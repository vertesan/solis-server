// Package live simulates performances from an immutable, fully resolved input.
// Persistence, authentication and reward settlement belong to the calling mode.
package live

import (
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

const RulesetVersion = "live-v5"

// CardData retains scoring identity independently of a visual replacement idol.
type CardData struct {
	Bonuses           map[penum.PhotoAbilityType]int64
	StrengthModifiers []StrengthModifier
	CharacterID       string
	GroupID           string
	Type              penum.CardType
	Attribute         penum.AttributeType
	LightFans         int64
	MiddleFans        int64
	HeavyFans         int64
	// CharacterAdvantagePermil scales the multiplied score, never prepared
	// attributes or fixed score additions. Zero is a neutral omitted value.
	CharacterAdvantagePermil int32
}

// StrengthModifier preserves the photo's status restriction. These modifiers
// must not be flattened into Bonuses, which would affect unrelated buffs.
type StrengthModifier struct {
	Type        penum.PhotoAbilityType
	StatusTypes []penum.StatusEffectType
	Amount      int32
}

// Input is resolved by the adapter once per transaction attempt. Simulate clones
// mutable protobuf state; callers may safely retain/reuse master definitions.
type Input struct {
	Quest              *pmaster.Quest
	Setting            *pmaster.Setting
	Chart              []*pmaster.MusicChartPattern
	Pressure           *pmaster.QuestPressure
	AudienceAdvantages []*pmaster.QuestAudienceAdvantage
	ComboAdvantages    []*pmaster.ComboAdvantage
	StaminaConsumption []*pmaster.QuestStaminaConsumption
	Users              []*papi.LiveUserInfo
	Cards              map[int32]CardData
	Skills             map[string]*pmaster.Skill
	Efficacies         map[string]*pmaster.SkillEfficacy
	Triggers           map[string]*pmaster.SkillTrigger
	Targets            map[string]*pmaster.SkillTarget
	Seed               uint64
}
