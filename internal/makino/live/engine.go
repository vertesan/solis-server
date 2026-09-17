package live

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"google.golang.org/protobuf/proto"
)

const (
	maxChartEvents     = 10000
	maxActorSkills     = 64
	maxStatusInstances = 1024
)

type actor struct {
	index           int32
	position        int
	side            int
	card            *papi.LiveDeckCard
	data            CardData
	synthetic       bool
	stamina         int64
	score           int64
	consumed        int64
	activations     int64
	skills          []*skillState
	effects         []*statusInstance
	migrations      []statusMigration
	revision        uint64
	cachedRevision  uint64
	cached          *papi.LiveCardStatus
	summary         map[penum.StatusEffectType]statusTotals
	summaryRevision uint64
	effectRows      map[penum.StatusEffectType]*papi.LiveCardStatusEffect
	effectSnapshot  []*papi.LiveCardStatusEffect
}
type sideState struct {
	info  *papi.LiveUserInfo
	combo int32
}
type eventState struct {
	chart          *pmaster.MusicChartPattern
	ordinal        int
	winner         *actor
	winningSkill   *skillState
	critical       map[int32]bool
	criticalRolls  map[int32]bool
	candidates     []activeCandidate
	recovered      map[int32]bool
	activated      map[int32]penum.SkillCategoryType
	triggerActor   *actor
	triggerActors  []*actor
	activeResolved bool
	budget         map[int64]bool
}
type simulation struct {
	input         *Input
	result        *papi.LiveResult
	rng           *rand.Rand
	actors        []*actor
	passiveActors []*actor
	byIndex       map[int32]*actor
	sides         []*sideState
	event         eventState
	order         int32
	occupancy     float64
	beatCount     int
	comboRows     []*pmaster.ComboAdvantage
	audienceRows  []*pmaster.QuestAudienceAdvantage
	staminaRows   []*pmaster.QuestStaminaConsumption
	err           error
	strengthTypes map[penum.StatusEffectType]bool
	weaknessTypes map[penum.StatusEffectType]bool
	reasons       map[[2]int32]*papi.LiveStatusEffectReason
}

// Simulate performs no database or filesystem operations. Each invocation owns
// its RNG and mutable state. A retry with the same input and seed is reproducible.
func Simulate(ctx context.Context, input *Input) (*papi.LiveResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e, err := newSimulation(input)
	if err != nil {
		return nil, err
	}
	e.event = eventState{ordinal: 0, critical: map[int32]bool{}, criticalRolls: map[int32]bool{}, recovered: map[int32]bool{}, activated: map[int32]penum.SkillCategoryType{}, budget: map[int64]bool{}}
	e.result.ActivatedPassiveSkills = e.passives("start")
	e.result.PreChart = &papi.LiveChart{CardStatuses: e.cardSnapshots(), UserStatuses: e.userSnapshots()}
	chart := append([]*pmaster.MusicChartPattern(nil), input.Chart...)
	e.result.Charts = make([]*papi.LiveChart, 0, len(chart))
	sort.SliceStable(chart, func(i, j int) bool { return chart[i].Number < chart[j].Number })
	previous := int32(-1)
	for _, step := range chart {
		if step.Type == penum.MusicChartType_MusicChartType_Unknown {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if step.Number <= previous {
			return nil, fmt.Errorf("live chart numbers must be unique and increasing")
		}
		previous = step.Number
		e.event.chart = step
		e.event.ordinal++
		e.event.winner, e.event.winningSkill, e.event.triggerActor = nil, nil, nil
		e.event.triggerActors = nil
		e.event.candidates = e.event.candidates[:0]
		e.event.activeResolved = false
		clear(e.event.critical)
		clear(e.event.criticalRolls)
		clear(e.event.recovered)
		clear(e.event.activated)
		clear(e.event.budget)
		row := &papi.LiveChart{Number: step.Number, ChartType: step.Type}
		var failures []*papi.LiveSkillFailure
		if step.Type != penum.MusicChartType_MusicChartType_Beat {
			row.AttributeType = e.laneAttribute(int(step.Position))
			failures = e.selectActive()
		}
		// Critical-trigger passives observe the same preselected roll that the
		// ensuing beat/skill uses; logging never consumes random numbers.
		if step.Type == penum.MusicChartType_MusicChartType_Beat {
			for _, a := range e.actors {
				if !a.synthetic {
					e.event.critical[a.index] = e.rng.Float64() < e.criticalRate(a)
				}
			}
		} else {
			for _, candidate := range e.event.candidates {
				a := candidate.actor
				e.event.criticalRolls[a.index] = e.rng.Float64() < e.criticalRate(a)
				if a == e.event.winner {
					e.event.critical[a.index] = e.event.criticalRolls[a.index]
				}
			}
		}
		row.ActivatedPassiveSkills = append(row.ActivatedPassiveSkills, e.passives("early")...)
		if step.Type == penum.MusicChartType_MusicChartType_Beat {
			for _, a := range e.actors {
				if a.synthetic {
					continue
				}
				score := e.beatScore(a, e.event.critical[a.index])
				e.addScore(a, score)
				if e.event.critical[a.index] {
					e.sides[a.side].info.UserResult.CriticalCount++
				}
				row.Beats = append(row.Beats, &papi.LiveBeat{Order: e.nextOrder(), CardIndex: a.index, Score: score, IsCritical: e.event.critical[a.index]})
			}
			for _, s := range e.sides {
				e.incrementCombo(s)
			}
		} else {
			row.ActivatedSkill = e.activeSkill(failures)
		}
		e.event.activeResolved = true
		e.tick()
		row.ActivatedPassiveSkills = append(row.ActivatedPassiveSkills, e.passives("late")...)
		if e.err != nil {
			return nil, e.err
		}
		row.CardStatuses = e.cardSnapshots()
		row.UserStatuses = e.userSnapshots()
		e.result.Charts = append(e.result.Charts, row)
	}
	if len(e.result.Charts) == 0 {
		return nil, fmt.Errorf("live has no playable chart events")
	}
	for _, a := range e.actors {
		if !a.synthetic {
			e.sides[a.side].info.UserResult.CardResults = append(e.sides[a.side].info.UserResult.CardResults, &papi.LiveCardResult{CardId: a.card.CardId, TotalScore: a.score})
		}
	}
	if len(e.sides) == 2 {
		e.result.Cleared = e.sides[0].info.UserResult.Score > e.sides[1].info.UserResult.Score
	} else {
		e.result.Cleared = e.sides[0].info.UserResult.Score >= input.Quest.ClearScore
	}
	e.result.IsDisplayDetailAvailable = true
	return e.result, nil
}

func newSimulation(in *Input) (*simulation, error) {
	if in == nil || in.Quest == nil || in.Setting == nil || in.Pressure == nil {
		return nil, fmt.Errorf("live input is incomplete")
	}
	if in.Quest.DifficultyLevel <= 0 || in.Quest.MaxCapacity <= 0 || in.Quest.MentalThreshold <= 0 || in.Pressure.WeightPermil <= 0 {
		return nil, fmt.Errorf("invalid live difficulty, capacity or pressure")
	}
	if len(in.Users) < 1 || len(in.Users) > 2 || len(in.Chart) == 0 || len(in.Chart) > maxChartEvents {
		return nil, fmt.Errorf("invalid live unit/chart size")
	}
	if len(in.AudienceAdvantages) == 0 || len(in.ComboAdvantages) == 0 || len(in.StaminaConsumption) == 0 {
		return nil, fmt.Errorf("live numeric tables are missing")
	}
	e := &simulation{input: in, result: &papi.LiveResult{}, rng: rand.New(rand.NewPCG(in.Seed, in.Seed^0x9e3779b97f4a7c15)), byIndex: make(map[int32]*actor), strengthTypes: make(map[penum.StatusEffectType]bool), weaknessTypes: make(map[penum.StatusEffectType]bool)}
	for _, t := range in.Setting.StatusEffectTypeStrengthList {
		e.strengthTypes[t] = true
	}
	for _, t := range in.Setting.StatusEffectTypeWeaknessDownList {
		e.weaknessTypes[t] = true
	}
	for _, t := range in.Setting.StatusEffectTypeWeaknessOtherList {
		e.weaknessTypes[t] = true
	}
	e.comboRows = append([]*pmaster.ComboAdvantage(nil), in.ComboAdvantages...)
	sort.Slice(e.comboRows, func(i, j int) bool { return e.comboRows[i].ComboCount < e.comboRows[j].ComboCount })
	e.audienceRows = append([]*pmaster.QuestAudienceAdvantage(nil), in.AudienceAdvantages...)
	sort.Slice(e.audienceRows, func(i, j int) bool { return e.audienceRows[i].AudienceAmount < e.audienceRows[j].AudienceAmount })
	e.staminaRows = append([]*pmaster.QuestStaminaConsumption(nil), in.StaminaConsumption...)
	sort.Slice(e.staminaRows, func(i, j int) bool {
		return e.staminaRows[i].RemainStaminaRatioPercent < e.staminaRows[j].RemainStaminaRatioPercent
	})
	for side, source := range in.Users {
		if source == nil || source.UserDeck == nil || len(source.UserDeck.Cards) != 5 {
			return nil, fmt.Errorf("a live unit must contain five cards")
		}
		u := proto.Clone(source).(*papi.LiveUserInfo)
		u.Index = int32(side + 1)
		u.UserResult = &papi.LiveUserResult{}
		e.sides = append(e.sides, &sideState{info: u})
		e.result.UserInfos = append(e.result.UserInfos, u)
		for _, c := range u.UserDeck.Cards {
			if c == nil || c.Index < int32(side*5+1) || c.Index > int32(side*5+5) || e.byIndex[c.Index] != nil {
				return nil, fmt.Errorf("invalid or duplicate live card index")
			}
			if c.CardId == "" || c.Stamina <= 0 || c.Mental < 0 || c.Technique < 0 || c.Vocal < 0 || c.Dance < 0 || c.Visual < 0 || c.AudienceAmount < 0 {
				return nil, fmt.Errorf("invalid prepared live card %q", c.CardId)
			}
			// Exact protobuf integer scores are bounded below int64's limit. This
			// also keeps floating-point score evaluation in its useful domain.
			for _, v := range []int64{c.Stamina, c.Vocal, c.Dance, c.Visual, c.Mental, c.Technique} {
				if v > 1_000_000_000_000 {
					return nil, fmt.Errorf("live card attribute exceeds supported range")
				}
			}
			a := &actor{index: c.Index, position: int((c.Index-1)%5 + 1), side: side, card: c, data: in.Cards[c.Index], stamina: c.Stamina, revision: 1}
			c.StartStamina = c.Stamina
			if a.data.CharacterID == "" {
				return nil, fmt.Errorf("live scoring character identity missing for card %q", c.CardId)
			}
			if a.data.CharacterAdvantagePermil < 0 {
				return nil, fmt.Errorf("negative live character advantage for card %q", c.CardId)
			}
			for typ, value := range a.data.Bonuses {
				if value < 0 {
					return nil, unsupported(c.CardId, "negative live equipment bonus")
				}
				if err := validateBonusType(c.CardId, typ, value); err != nil {
					return nil, err
				}
			}
			if err := e.addActor(a, c.Skills); err != nil {
				return nil, err
			}
			if int64(e.result.TotalAudienceAmount)+int64(c.AudienceAmount) > int64(in.Quest.MaxCapacity) {
				return nil, fmt.Errorf("audience exceeds venue capacity")
			}
			e.result.TotalAudienceAmount += c.AudienceAmount
		}
		if len(u.UserDeck.BonusSkills) > 0 {
			a := &actor{index: int32(999 + side), side: side, position: 1, synthetic: true, card: &papi.LiveDeckCard{}, revision: 1}
			if err := e.addActor(a, u.UserDeck.BonusSkills); err != nil {
				return nil, err
			}
		}
	}
	for _, step := range in.Chart {
		if step == nil || step.Number < 0 {
			return nil, fmt.Errorf("invalid master live chart row")
		}
		switch step.Type {
		case penum.MusicChartType_MusicChartType_Unknown:
		case penum.MusicChartType_MusicChartType_Beat:
			// Every normal chart beat awards one beat score to every lane. Skill
			// opportunities and empty musical positions are not normal beats.
			e.beatCount++
		case penum.MusicChartType_MusicChartType_ActiveSkill, penum.MusicChartType_MusicChartType_SpecialSkill:
			if step.Position < 1 || step.Position > 5 {
				return nil, fmt.Errorf("invalid skill chance position")
			}
		default:
			return nil, fmt.Errorf("unsupported master live chart type %s", step.Type)
		}
	}
	e.occupancy = float64(e.result.TotalAudienceAmount) / float64(in.Quest.MaxCapacity)
	sort.Slice(e.actors, func(i, j int) bool { return e.actors[i].index < e.actors[j].index })
	e.passiveActors = append([]*actor(nil), e.actors...)
	sort.SliceStable(e.passiveActors, func(i, j int) bool {
		a, b := e.passiveActors[i], e.passiveActors[j]
		if a.synthetic != b.synthetic {
			return a.synthetic
		}
		if a.card.Mental != b.card.Mental {
			return a.card.Mental > b.card.Mental
		}
		return actorBefore(a, b)
	})
	return e, nil
}

func (e *simulation) addActor(a *actor, refs []*papi.LiveDeckCardSkill) error {
	if len(refs) > maxActorSkills {
		return fmt.Errorf("too many live skills")
	}
	seen := map[int32]bool{}
	for _, ref := range refs {
		if ref == nil || seen[ref.Index] {
			return fmt.Errorf("nil or duplicate live skill index")
		}
		seen[ref.Index] = true
		s, err := compileSkill(e.input, ref)
		if err != nil {
			return err
		}
		if a.synthetic && s.definition.CategoryType != penum.SkillCategoryType_SkillCategoryType_Passive {
			return unsupported(ref.SkillId, "stage bonus is not passive")
		}
		a.skills = append(a.skills, s)
	}
	e.actors = append(e.actors, a)
	e.byIndex[a.index] = a
	return nil
}

func (e *simulation) nextOrder() int32 { e.order++; return e.order }
func (e *simulation) incrementCombo(s *sideState) {
	s.combo++
	if s.combo > s.info.UserResult.MaxComboCount {
		s.info.UserResult.MaxComboCount = s.combo
	}
}
func (e *simulation) addScore(a *actor, value int64) {
	if value < 0 || a.score > math.MaxInt64-value || e.sides[a.side].info.UserResult.Score > math.MaxInt64-value {
		e.err = fmt.Errorf("live score overflow")
		return
	}
	a.score += value
	e.sides[a.side].info.UserResult.Score += value
}
func (e *simulation) scoreValue(v float64) int64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > float64(math.MaxInt64/4) {
		e.err = fmt.Errorf("invalid live score calculation")
		return 0
	}
	return int64(math.Floor(v))
}

func (e *simulation) userSnapshots() []*papi.LiveUserStatus {
	out := make([]*papi.LiveUserStatus, 0, len(e.sides))
	for _, s := range e.sides {
		out = append(out, &papi.LiveUserStatus{UserIndex: s.info.Index, CurrentComboCount: s.combo, TotalScore: s.info.UserResult.Score})
	}
	return out
}
func (e *simulation) cardSnapshots() []*papi.LiveCardStatus {
	out := make([]*papi.LiveCardStatus, 0, len(e.actors))
	for _, a := range e.actors {
		if a.synthetic {
			continue
		}
		if a.cached == nil || a.cachedRevision != a.revision {
			s := &papi.LiveCardStatus{CardIndex: a.index, Stamina: a.stamina, Vocal: e.attribute(a, penum.AttributeType_AttributeType_Vocal), Dance: e.attribute(a, penum.AttributeType_AttributeType_Dance), Visual: e.attribute(a, penum.AttributeType_AttributeType_Visual), Effects: e.statusSnapshots(a)}
			for _, skill := range a.skills {
				s.SkillStatuses = append(s.SkillStatuses, &papi.LiveSkillStatus{SkillIndex: skill.ref.Index, RemainCount: skill.remaining, CoolTime: skill.cooldown})
			}
			a.cached = s
			a.cachedRevision = a.revision
		}
		// Cached entries are immutable. Reuse across snapshots rather than
		// copying unchanged card/skill/effect state for every passive evaluation.
		out = append(out, a.cached)
	}
	return out
}

func (e *simulation) laneAttribute(position int) penum.AttributeType {
	switch position {
	case 1:
		return e.input.Quest.Position1AttributeType
	case 2:
		return e.input.Quest.Position2AttributeType
	case 3:
		return e.input.Quest.Position3AttributeType
	case 4:
		return e.input.Quest.Position4AttributeType
	case 5:
		return e.input.Quest.Position5AttributeType
	}
	return penum.AttributeType_AttributeType_Unknown
}

func validateBonusType(cardID string, typ penum.PhotoAbilityType, value int64) error {
	switch typ {
	case penum.PhotoAbilityType_PhotoAbilityType_DanceAdd, penum.PhotoAbilityType_PhotoAbilityType_DanceMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_VocalAdd, penum.PhotoAbilityType_PhotoAbilityType_VocalMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_VisualAdd, penum.PhotoAbilityType_PhotoAbilityType_VisualMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_StaminaAdd, penum.PhotoAbilityType_PhotoAbilityType_StaminaMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_MentalAdd, penum.PhotoAbilityType_PhotoAbilityType_MentalMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_TechniqueAdd, penum.PhotoAbilityType_PhotoAbilityType_TechniqueMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_ManagerExp, penum.PhotoAbilityType_PhotoAbilityType_Gold, penum.PhotoAbilityType_PhotoAbilityType_CardExp,
		penum.PhotoAbilityType_PhotoAbilityType_BeatScoreAdd, penum.PhotoAbilityType_PhotoAbilityType_BeatScoreMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreAdd, penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreAdd, penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_CriticalScoreMultiply, penum.PhotoAbilityType_PhotoAbilityType_AudienceAmountUp,
		penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill1, penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill2, penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill3:
		return nil
	case penum.PhotoAbilityType_PhotoAbilityType_ReceivedStrengthEffectValueIncrease, penum.PhotoAbilityType_PhotoAbilityType_ReceivedStrengthEffectCountIncrease,
		penum.PhotoAbilityType_PhotoAbilityType_AffectStrengthEffectValueIncrease, penum.PhotoAbilityType_PhotoAbilityType_AffectStrengthEffectCountIncrease:
		if value != 0 {
			return unsupported(cardID, "status-specific photo effects must use typed StrengthModifiers")
		}
		return nil
	case penum.PhotoAbilityType_PhotoAbilityType_Unknown, penum.PhotoAbilityType_PhotoAbilityType_PassiveSkill:
		if value == 0 {
			return nil
		}
	}
	return unsupported(cardID, "unrecognized or unprepared photo bonus "+typ.String())
}
