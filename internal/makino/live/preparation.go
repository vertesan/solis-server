package live

import (
	"embed"
	"fmt"
	"math"
	"math/big"
	"slices"
	"sort"
	"sync"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.yaml.in/yaml/v3"
)

// Embedded files are read into temporary buffers; only decoded typed settings
// are retained, never a second global copy of their raw YAML bytes.
//
//go:embed npc_balance.yaml npc_rosters.yaml
var preparationFiles embed.FS

type npcCurve struct {
	Base    int64 `yaml:"baseAddition"`
	Growth  int64 `yaml:"growthAtAnchor"`
	Minimum int64 `yaml:"minimumThresholdPermil"`
}
type npcBalanceConfig struct {
	Version   string   `yaml:"version"`
	Start     int64    `yaml:"difficultyStart"`
	Anchor    int64    `yaml:"difficultyAnchor"`
	Tail      string   `yaml:"growthAfterAnchor"`
	Primary   npcCurve `yaml:"primary"`
	Stamina   npcCurve `yaml:"stamina"`
	Mental    npcCurve `yaml:"mental"`
	Technique npcCurve `yaml:"technique"`
}

var loadNPCBalance = sync.OnceValues(func() (*npcBalanceConfig, error) {
	raw, err := preparationFiles.ReadFile("npc_balance.yaml")
	if err != nil {
		return nil, err
	}
	var c npcBalanceConfig
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if c.Version == "" || c.Start < 0 || c.Anchor <= c.Start || c.Tail != "linear-tangent" {
		return nil, fmt.Errorf("invalid NPC balance configuration")
	}
	for _, curve := range []npcCurve{c.Primary, c.Stamina, c.Mental, c.Technique} {
		if curve.Base < 0 || curve.Growth < 0 || curve.Minimum < 0 {
			return nil, fmt.Errorf("negative NPC balance coefficient")
		}
	}
	return &c, nil
})

type NPCBonuses struct {
	Version                             string
	Primary, Stamina, Mental, Technique int64
}

// NPCBalance implements the authored v1 curve for every new NPC simulation.
// Mental and Technique are final values; Primary and Stamina are additions.
func NPCBalance(q *pmaster.Quest, setting *pmaster.Setting) (NPCBonuses, error) {
	var out NPCBonuses
	if q.GetDifficultyLevel() <= 0 || q.GetMentalThreshold() <= 0 || q.GetMaxCapacity() <= 0 || setting.GetCardDefaultMental() <= 0 || setting.GetCardDefaultTechnique() <= 0 {
		return out, fmt.Errorf("invalid NPC quest/default parameters")
	}
	c, err := loadNPCBalance()
	if err != nil {
		return out, err
	}
	x, h := max(0, int64(q.GetDifficultyLevel())-c.Start), c.Anchor-c.Start
	// big.Int keeps edited coefficients safe; outputs must still fit int64.
	denominator := new(big.Int).Mul(big.NewInt(h), big.NewInt(h))
	numerator := new(big.Int).Mul(big.NewInt(x), big.NewInt(x))
	if x > h {
		tail := new(big.Int).Mul(big.NewInt(2), big.NewInt(h))
		tail.Mul(tail, big.NewInt(x-h))
		numerator.Add(denominator, tail)
	}
	calc := func(curve npcCurve, base int64) (int64, error) {
		n := new(big.Int).Mul(numerator, big.NewInt(curve.Growth))
		n.Quo(n, denominator)
		n.Add(n, big.NewInt(curve.Base))
		n.Add(n, big.NewInt(base))
		if !n.IsInt64() || n.Sign() < 0 {
			return 0, fmt.Errorf("NPC balance overflows int64")
		}
		return n.Int64(), nil
	}
	out.Version = c.Version
	if out.Primary, err = calc(c.Primary, 0); err != nil {
		return out, err
	}
	if out.Stamina, err = calc(c.Stamina, 0); err != nil {
		return out, err
	}
	if out.Mental, err = calc(c.Mental, int64(setting.GetCardDefaultMental())); err != nil {
		return out, err
	}
	if out.Technique, err = calc(c.Technique, int64(setting.GetCardDefaultTechnique())); err != nil {
		return out, err
	}
	n := new(big.Int).Mul(big.NewInt(int64(q.GetMentalThreshold())), big.NewInt(c.Mental.Minimum))
	n.Add(n, big.NewInt(999))
	n.Quo(n, big.NewInt(1000))
	if !n.IsInt64() {
		return out, fmt.Errorf("NPC mental threshold overflows int64")
	}
	out.Mental = max(out.Mental, n.Int64())
	return out, nil
}

type npcRosterCard struct {
	CardID    string `yaml:"cardId"`
	Position  int32  `yaml:"position"`
	Level     int32  `yaml:"level"`
	Rarity    int32  `yaml:"rarity"`
	Light     int32  `yaml:"lightFanAmount"`
	Middle    int32  `yaml:"middleFanAmount"`
	Heavy     int32  `yaml:"heavyFanAmount"`
	Evolution int32  `yaml:"evolutionLevel"`
}
type npcRoster struct {
	Name         string          `yaml:"name"`
	ManagerLevel int32           `yaml:"managerLevel"`
	Cards        []npcRosterCard `yaml:"cards"`
}

var loadNPCRosters = sync.OnceValues(func() (map[string]npcRoster, error) {
	raw, err := preparationFiles.ReadFile("npc_rosters.yaml")
	if err != nil {
		return nil, err
	}
	var data struct {
		Rosters map[string]npcRoster `yaml:"rosters"`
	}
	if err := yaml.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data.Rosters, nil
})

// NPCRoster returns a fresh protobuf so callers cannot mutate shared settings.
func NPCRoster(questID string) (*papi.QuestOpponentInfo, error) {
	rows, err := loadNPCRosters()
	if err != nil {
		return nil, err
	}
	r, ok := rows[questID]
	if !ok {
		return nil, fmt.Errorf("NPC roster for %q is not configured", questID)
	}
	out := &papi.QuestOpponentInfo{Name: r.Name, ManagerLevel: r.ManagerLevel}
	seen := map[int32]bool{}
	for _, c := range r.Cards {
		if c.Position < 1 || c.Position > 5 || seen[c.Position] || c.CardID == "" || c.Level <= 0 || c.Rarity <= 0 || c.Light < 0 || c.Middle < 0 || c.Heavy < 0 {
			return nil, fmt.Errorf("invalid NPC roster %q", questID)
		}
		seen[c.Position] = true
		out.CardInfos = append(out.CardInfos, &papi.LiveBattleCardInfo{Position: c.Position, CardId: c.CardID, Level: c.Level, Rarity: c.Rarity, LightFanAmount: c.Light, MiddleFanAmount: c.Middle, HeavyFanAmount: c.Heavy, EvolutionLevel: c.Evolution})
	}
	if len(out.CardInfos) != 5 {
		return nil, fmt.Errorf("NPC roster requires five cards")
	}
	return out, nil
}

// AllocateAudience floors fan tiers separately, applies the candidate minimum,
// then ceilings proportional seats. Remove overbooking from the largest lane,
// choosing the latest lane on ties (the documented capture-backed policy).
func AllocateAudience(cards []CardData, capacity int32, setting *pmaster.Setting) ([]int32, error) {
	if capacity <= 0 || len(cards) == 0 || len(cards) > 10 || setting.GetLiveMinimumGuaranteedAudienceAmount() < 0 {
		return nil, fmt.Errorf("invalid venue or actor count")
	}
	weights := []int64{int64(setting.GetLiveLightFanAmountWeightPermil()), int64(setting.GetLiveMiddleFanAmountWeightPermil()), int64(setting.GetLiveHeavyFanAmountWeightPermil())}
	candidates := make([]int64, len(cards))
	var total int64
	for i, c := range cards {
		var n int64
		for j, f := range []int64{c.LightFans, c.MiddleFans, c.HeavyFans} {
			if f < 0 || weights[j] < 0 || weights[j] != 0 && f > math.MaxInt64/weights[j] {
				return nil, fmt.Errorf("invalid fan count or weight")
			}
			v := f * weights[j] / 1000
			if n > math.MaxInt64-v {
				return nil, fmt.Errorf("audience overflow")
			}
			n += v
		}
		n = max(n, int64(setting.GetLiveMinimumGuaranteedAudienceAmount()))
		// No current master record specifies this option's units. AmountUp is
		// interpreted as additional candidate fans (not a multiplier), before
		// both sides share the same capacity allocation.
		bonus := c.Bonuses[penum.PhotoAbilityType_PhotoAbilityType_AudienceAmountUp]
		if bonus < 0 || n > math.MaxInt64-bonus {
			return nil, fmt.Errorf("invalid audience bonus")
		}
		n += bonus
		if n < 0 || total > math.MaxInt64-n {
			return nil, fmt.Errorf("invalid audience minimum or total")
		}
		candidates[i] = n
		total += n
	}
	out := make([]int32, len(cards))
	if total == 0 {
		return out, nil
	}
	var assigned int64
	for i, n := range candidates {
		seats := n
		if total > int64(capacity) {
			v := new(big.Int).Mul(big.NewInt(n), big.NewInt(int64(capacity)))
			v.Add(v, big.NewInt(total-1))
			v.Quo(v, big.NewInt(total))
			seats = v.Int64()
		}
		if seats > math.MaxInt32 {
			return nil, fmt.Errorf("audience exceeds protocol capacity")
		}
		out[i] = int32(seats)
		assigned += seats
	}
	for assigned > int64(capacity) {
		largest := 0
		for i := range out {
			if out[i] >= out[largest] {
				largest = i
			}
		}
		out[largest]--
		assigned--
	}
	return out, nil
}

// ResolveCharacterAdvantage returns the encounter's score multiplier for the
// scoring character, independently of any cosmetic replacement. It must not
// alter prepared stats: attribute-based targeting and battle authority use
// those stats without the score-only advantage. Master 1000 is neutral.
func ResolveCharacterAdvantage(data CardData, advantage *pmaster.QuestCharacterAdvantage) (int32, error) {
	if advantage == nil {
		return 1000, nil
	}
	if !slices.Contains(advantage.GetCharacterIds(), data.CharacterID) && (advantage.GetCharacterGroupId() == "" || advantage.GetCharacterGroupId() != data.GroupID) {
		return 1000, nil
	}
	if advantage.GetAdvantagePermil() <= 0 {
		return 0, fmt.Errorf("invalid character advantage")
	}
	return advantage.GetAdvantagePermil(), nil
}

// PhotoTargetIndexes resolves equipment recipients within one side only.
// The source photo's card/live conditions are checked before this function.
func PhotoTargetIndexes(cards []*papi.LiveDeckCard, data map[int32]CardData, source int32, target *pmaster.PhotoAbilityTarget) ([]int32, error) {
	if target == nil {
		return []int32{source}, nil
	}
	var sourcePosition int = -1
	for _, card := range cards {
		if card.GetIndex() == source {
			sourcePosition = int((card.GetIndex()-1)%5 + 1)
			break
		}
	}
	if sourcePosition < 0 {
		return nil, fmt.Errorf("photo source is not in this unit")
	}
	var result []int32
	for _, card := range cards {
		position := int((card.GetIndex()-1)%5 + 1)
		match := false
		switch target.GetType() {
		case penum.PhotoAbilityTargetType_PhotoAbilityTargetType_Self:
			match = card.GetIndex() == source
		case penum.PhotoAbilityTargetType_PhotoAbilityTargetType_All:
			match = true
		case penum.PhotoAbilityTargetType_PhotoAbilityTargetType_Center:
			match = position == 1
		case penum.PhotoAbilityTargetType_PhotoAbilityTargetType_Neighbor:
			match = lanesAreNeighbors(position, sourcePosition)
		case penum.PhotoAbilityTargetType_PhotoAbilityTargetType_Character:
			match = target.GetTargetId() != "" && data[card.GetIndex()].CharacterID == target.GetTargetId()
		case penum.PhotoAbilityTargetType_PhotoAbilityTargetType_CharacterGroup:
			match = target.GetTargetId() != "" && data[card.GetIndex()].GroupID == target.GetTargetId()
		default:
			return nil, fmt.Errorf("unsupported photo target %v", target.GetType())
		}
		if match {
			result = append(result, card.GetIndex())
		}
	}
	return result, nil
}

// ExtraCharacterIDs chooses cosmetic performers independently of the five
// scoring actors. Five-performer songs retain the captured optional sixth spare.
func ExtraCharacterIDs(displayCount int32, owned, displayed []string) ([]string, error) {
	if displayCount < 0 {
		return nil, fmt.Errorf("negative display character count")
	}
	used := make(map[string]bool, len(owned))
	for _, id := range displayed {
		used[id] = true
	}
	spares := make([]string, 0, len(owned))
	for _, id := range owned {
		if id != "" && !used[id] {
			spares = append(spares, id)
			used[id] = true
		}
	}
	sort.Strings(spares)
	required := max(0, int(displayCount)-len(displayed))
	if required > len(spares) {
		return nil, fmt.Errorf("music requires %d additional owned display idols", required)
	}
	count := min(len(spares), max(1, required))
	return spares[:count], nil
}
