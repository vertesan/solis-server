package live

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
)

type statusInstance struct {
	spec          effectSpec
	grade         int32
	remaining     int32
	source, skill int32
}

type statusTotals struct{ sum, cap, limit int32 }

var limitStatuses = relatedStatuses("limit_break_")
var superStatuses = relatedStatuses("add_effect_value_")

// Temporary user-selected balance rule pending an official BeatBoost capture.
const beatBoostPermilPerStage int64 = 50

func relatedStatuses(prefix string) map[penum.StatusEffectType]penum.StatusEffectType {
	out := make(map[penum.StatusEffectType]penum.StatusEffectType)
	for name, value := range statusNames {
		if strings.HasPrefix(name, prefix) {
			if base, ok := statusNames[strings.TrimPrefix(name, prefix)]; ok {
				out[base] = value
			}
		}
	}
	return out
}

func (e *simulation) rawGrade(a *actor, status penum.StatusEffectType) (sum, cap, limit int32) {
	if len(a.effects) == 0 {
		return 0, 0, 0
	}
	if a.summary == nil || a.summaryRevision != a.revision {
		if a.summary == nil {
			a.summary = make(map[penum.StatusEffectType]statusTotals)
		} else {
			clear(a.summary)
		}
		for _, s := range a.effects {
			t := a.summary[s.spec.status]
			t.sum += s.grade
			t.cap = max(t.cap, s.spec.cap)
			t.limit = max(t.limit, s.spec.limitCap)
			a.summary[s.spec.status] = t
		}
		a.summaryRevision = a.revision
	}
	t := a.summary[status]
	return t.sum, t.cap, t.limit
}
func (e *simulation) grade(a *actor, status penum.StatusEffectType) int32 {
	if status == penum.StatusEffectType_StatusEffectType_Unknown {
		return 0
	}
	sum, cap, limit := e.rawGrade(a, status)
	if cap == 0 {
		return 0
	}
	if extension, ok := limitStatuses[status]; ok {
		v, c, _ := e.rawGrade(a, extension)
		cap += min(v, c)
		if limit > 0 {
			cap = min(cap, limit)
		}
	}
	if e.isStrength(status) {
		v, c, _ := e.rawGrade(a, penum.StatusEffectType_StatusEffectType_StrengthEffectGradeLimit)
		// Each limiting stage removes 5% of the normally available stages.
		cap = int32(math.Floor(float64(cap) * max(0, 1-.05*float64(min(v, c)))))
	}
	return min(sum, cap)
}

func statusCoefficient(status penum.StatusEffectType) int64 {
	switch status {
	case penum.StatusEffectType_StatusEffectType_ScoreUp:
		return 25
	case penum.StatusEffectType_StatusEffectType_BeatBoost:
		return beatBoostPermilPerStage
	case penum.StatusEffectType_StatusEffectType_BeatScoreUp, penum.StatusEffectType_StatusEffectType_ComboScoreUp, penum.StatusEffectType_StatusEffectType_PassiveSkillScoreUp, penum.StatusEffectType_StatusEffectType_PassiveScoreMultiplierAdd:
		return 100
	case penum.StatusEffectType_StatusEffectType_SpecialSkillScoreUp:
		return 30
	case penum.StatusEffectType_StatusEffectType_VocalBoost, penum.StatusEffectType_StatusEffectType_DanceBoost, penum.StatusEffectType_StatusEffectType_VisualBoost:
		return 75
	case penum.StatusEffectType_StatusEffectType_SpecialScoreMultiplierAdd:
		return 400
	case penum.StatusEffectType_StatusEffectType_ActiveScoreMultiplierAdd:
		return 150
	case penum.StatusEffectType_StatusEffectType_StaminaContinuousRecovery, penum.StatusEffectType_StatusEffectType_StaminaContinuousConsumption:
		return 15
	case penum.StatusEffectType_StatusEffectType_ComboContinuation, penum.StatusEffectType_StatusEffectType_ActiveSkillChanceAssignment, penum.StatusEffectType_StatusEffectType_CoverWeaknessEffect, penum.StatusEffectType_StatusEffectType_WeaknessEffectReflection, penum.StatusEffectType_StatusEffectType_ScoreGetByStrengthEffectCountLimitUp, penum.StatusEffectType_StatusEffectType_SkillImpossible, penum.StatusEffectType_StatusEffectType_PassiveSkillImpossible:
		return 1
	case penum.StatusEffectType_StatusEffectType_AddEffectValueScoreUp, penum.StatusEffectType_StatusEffectType_AddEffectValueDanceUp, penum.StatusEffectType_StatusEffectType_AddEffectValueVocalUp, penum.StatusEffectType_StatusEffectType_AddEffectValueVisualUp, penum.StatusEffectType_StatusEffectType_AddEffectValueBeatScoreUp, penum.StatusEffectType_StatusEffectType_AddEffectValueActiveSkillScoreUp, penum.StatusEffectType_StatusEffectType_AddEffectValueCriticalBonusPermilUp, penum.StatusEffectType_StatusEffectType_AddEffectValueAudienceAmountIncrease, penum.StatusEffectType_StatusEffectType_AddEffectValueSpecialSkillScoreUp, penum.StatusEffectType_StatusEffectType_AddEffectValueSkillSuccessRateUp, penum.StatusEffectType_StatusEffectType_AddEffectValuePassiveSkillScoreUp, penum.StatusEffectType_StatusEffectType_AddEffectValueAudienceAmountReduction, penum.StatusEffectType_StatusEffectType_AddEffectValueTensionUp, penum.StatusEffectType_StatusEffectType_AddEffectValueDanceDown, penum.StatusEffectType_StatusEffectType_AddEffectValueVocalDown, penum.StatusEffectType_StatusEffectType_AddEffectValueVisualDown:
		return 25
	default:
		return 50
	}
}
func (e *simulation) effectValue(a *actor, status penum.StatusEffectType) int64 {
	g := e.grade(a, status)
	if g == 0 {
		return 0
	}
	v := int64(g) * statusCoefficient(status)
	if super, ok := superStatuses[status]; ok {
		v += int64(e.grade(a, super)) * statusCoefficient(super)
	}
	return v
}
func (e *simulation) attribute(a *actor, typ penum.AttributeType) int64 {
	var base int64
	var up, down, boost penum.StatusEffectType
	switch typ {
	case penum.AttributeType_AttributeType_Dance:
		base = a.card.Dance
		up = penum.StatusEffectType_StatusEffectType_DanceUp
		down = penum.StatusEffectType_StatusEffectType_DanceDown
		boost = penum.StatusEffectType_StatusEffectType_DanceBoost
	case penum.AttributeType_AttributeType_Vocal:
		base = a.card.Vocal
		up = penum.StatusEffectType_StatusEffectType_VocalUp
		down = penum.StatusEffectType_StatusEffectType_VocalDown
		boost = penum.StatusEffectType_StatusEffectType_VocalBoost
	case penum.AttributeType_AttributeType_Visual:
		base = a.card.Visual
		up = penum.StatusEffectType_StatusEffectType_VisualUp
		down = penum.StatusEffectType_StatusEffectType_VisualDown
		boost = penum.StatusEffectType_StatusEffectType_VisualBoost
	default:
		return 0
	}
	permil := int64(1000) + e.effectValue(a, up) - e.effectValue(a, down) + e.effectValue(a, boost)
	limit := int64(e.input.Setting.LiveMaxParameterBonusPermil)
	if limit > 0 {
		permil = min(permil, 1000+limit)
	}
	return e.scoreValue(float64(base) * float64(max(0, permil)) / 1000)
}

func (e *simulation) statusSnapshots(a *actor) []*papi.LiveCardStatusEffect {
	if len(a.effects) == 0 {
		return nil
	}
	var typeScratch [128]int
	types := typeScratch[:0]
	seen := map[penum.StatusEffectType]bool{}
	for _, s := range a.effects {
		if !seen[s.spec.status] {
			seen[s.spec.status] = true
			types = append(types, int(s.spec.status))
		}
	}
	sort.Ints(types)
	if a.effectRows == nil {
		a.effectRows = make(map[penum.StatusEffectType]*papi.LiveCardStatusEffect)
	}
	var rowScratch [128]*papi.LiveCardStatusEffect
	out := rowScratch[:0]
	for _, t := range types {
		typ := penum.StatusEffectType(t)
		g := e.grade(a, typ)
		value := e.effectValue(a, typ)
		var efficacy penum.SkillEfficacyType
		var maxGrade, limitGrade, remaining int32
		var reasonScratch [128]*papi.LiveStatusEffectReason
		reasons := reasonScratch[:0]
		for _, s := range a.effects {
			if s.spec.status != typ {
				continue
			}
			efficacy = s.spec.typ
			maxGrade = max(maxGrade, s.spec.cap)
			limitGrade = max(limitGrade, s.spec.limitCap)
			remaining = max(remaining, s.remaining)
			key := [2]int32{s.source, s.skill}
			if e.reasons == nil {
				e.reasons = make(map[[2]int32]*papi.LiveStatusEffectReason)
			}
			reason := e.reasons[key]
			if reason == nil {
				reason = &papi.LiveStatusEffectReason{CardIndex: s.source, SkillIndex: s.skill}
				e.reasons[key] = reason
			}
			duplicate := false
			for _, r := range reasons {
				if r == reason {
					duplicate = true
					break
				}
			}
			if !duplicate {
				reasons = append(reasons, reason)
			}
		}
		value2 := int64(0)
		if typ == penum.StatusEffectType_StatusEffectType_SkillSuccessRateUp {
			value2 = int64(g) * 375 / 10
		}
		if typ == penum.StatusEffectType_StatusEffectType_TensionUp {
			value2 = int64(g) * 15
		}
		row := a.effectRows[typ]
		same := row != nil && row.Grade == g && row.Value == value && row.Value2 == value2 && row.SkillEfficacyType == efficacy && row.MaxGrade == maxGrade && row.LimitBreakMaxGrade == limitGrade && row.RemainEffectedChartCount == remaining && len(row.StatusReasons) == len(reasons)
		if same {
			for i, r := range reasons {
				if row.StatusReasons[i] != r {
					same = false
					break
				}
			}
		}
		if !same {
			row = &papi.LiveCardStatusEffect{StatusType: typ, Grade: g, Value: value, Value2: value2, SkillEfficacyType: efficacy, MaxGrade: maxGrade, LimitBreakMaxGrade: limitGrade, RemainEffectedChartCount: remaining, StatusReasons: append([]*papi.LiveStatusEffectReason(nil), reasons...)}
			a.effectRows[typ] = row
		}
		out = append(out, row)
	}
	same := len(out) == len(a.effectSnapshot)
	if same {
		for i, r := range out {
			if a.effectSnapshot[i] != r {
				same = false
				break
			}
		}
	}
	if !same {
		a.effectSnapshot = append([]*papi.LiveCardStatusEffect(nil), out...)
	}
	return a.effectSnapshot
}

func (e *simulation) addStatus(target, source *actor, skill *skillState, spec effectSpec) {
	if len(target.effects)+len(target.migrations) >= maxStatusInstances {
		e.err = fmt.Errorf("too many concurrent live status instances")
		return
	}
	grade := spec.grade
	duration := spec.duration
	if e.isStrength(spec.status) {
		// Photo bonuses affect matching status applications only. Received
		// modifiers belong to the recipient; applied modifiers to the caster.
		// Relocation/transfer keeps the already-modified instance and must not
		// call this path a second time.
		apply := func(modifiers []StrengthModifier, received bool) {
			for _, modifier := range modifiers {
				matches := len(modifier.StatusTypes) == 0
				for _, status := range modifier.StatusTypes {
					matches = matches || status == spec.status
				}
				if !matches {
					continue
				}
				switch modifier.Type {
				case penum.PhotoAbilityType_PhotoAbilityType_ReceivedStrengthEffectValueIncrease:
					if received {
						grade += modifier.Amount
					}
				case penum.PhotoAbilityType_PhotoAbilityType_ReceivedStrengthEffectCountIncrease:
					if received {
						duration += modifier.Amount
					}
				case penum.PhotoAbilityType_PhotoAbilityType_AffectStrengthEffectValueIncrease:
					if !received {
						grade += modifier.Amount
					}
				case penum.PhotoAbilityType_PhotoAbilityType_AffectStrengthEffectCountIncrease:
					if !received {
						duration += modifier.Amount
					}
				}
			}
		}
		apply(source.data.StrengthModifiers, false)
		apply(target.data.StrengthModifiers, true)
	}
	target.effects = append(target.effects, &statusInstance{spec: spec, grade: grade, remaining: duration, source: source.index, skill: skill.ref.Index})
	target.revision++
}

func (e *simulation) changeStamina(a *actor, delta int64) int64 {
	before := a.stamina
	if delta >= 0 {
		a.stamina += min(delta, a.card.Stamina-a.stamina)
	} else {
		a.stamina = max(0, a.stamina+delta)
	}
	change := a.stamina - before
	if change < 0 {
		a.consumed -= change
	}
	if change > 0 {
		e.event.recovered[a.index] = true
	}
	if change != 0 {
		a.revision++
	}
	return change
}

func (e *simulation) tick() {
	for _, a := range e.actors {
		changed := false
		for _, s := range a.skills {
			if s.cooldown > 0 {
				s.cooldown--
				changed = true
			}
		}
		// Periodic effects apply on their final active event, before expiry.
		if !a.synthetic {
			recover := e.effectValue(a, penum.StatusEffectType_StatusEffectType_StaminaContinuousRecovery) * e.recoveryWeight() / 1000
			drain := e.effectValue(a, penum.StatusEffectType_StatusEffectType_StaminaContinuousConsumption)
			// BeatBoost has no stamina drain under the user's temporary rule.
			if recover != 0 {
				e.changeStamina(a, recover)
			}
			if drain != 0 {
				e.changeStamina(a, -drain)
			}
		}
		kept := a.effects[:0]
		for _, s := range a.effects {
			s.remaining--
			changed = true
			if s.remaining > 0 {
				kept = append(kept, s)
			}
		}
		// Drop references to expired instances rather than retaining them in the
		// unused backing-array tail until the whole simulation finishes.
		clear(a.effects[len(kept):])
		a.effects = kept
		if changed {
			a.revision++
		}
	}
}
