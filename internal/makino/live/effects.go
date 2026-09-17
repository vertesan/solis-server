package live

import (
	"fmt"
	"strings"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
)

type effectHandler func(*simulation, *actor, *actor, *skillState, effectSpec) int64

// New effect families add one compiler rule and one executor; ordinary statuses
// and parameter variants share these handlers rather than one class per skill.
var effectHandlers = map[string]effectHandler{
	"status":                                         applyStatus,
	"fix_stamina_recovery":                           applyRecovery,
	"target_stamina_recovery":                        applyRecovery,
	"stamina_consumption":                            applyDrain,
	"weakness_effect_recovery":                       applyStatusMutation,
	"weakness_effect_inversion":                      applyStatusMutation,
	"strength_effect_count_increase":                 applyStatusMutation,
	"strength_effect_value_increase":                 applyStatusMutation,
	"strength_effect_value_decrease":                 applyStatusMutation,
	"strength_effect_erasing":                        applyStatusMutation,
	"strength_effect_erasing_all":                    applyStatusMutation,
	"status_effect_change":                           applyStatusMutation,
	"cool_time_reduction":                            applyCooldown,
	"cool_time_increase":                             applyCooldown,
	"passive_skill_cool_time_reset":                  applyCooldown,
	"live_ability_cool_time_reduction":               applyCooldown,
	"strength_effect_migration_before_active_skill":  applyMigration,
	"strength_effect_migration_before_special_skill": applyMigration,
}

func (e *simulation) executeEffects(source *actor, skill *skillState, critical bool) ([]*papi.LiveSkillDetail, int64, bool) {
	var details []*papi.LiveSkillDetail
	var total int64
	matched := false
	triggerActor := e.event.triggerActor
	triggerActors := e.event.triggerActors
	for i, eff := range skill.effects {
		ok, bound := e.matchTrigger(eff.trigger, source)
		if !ok {
			continue
		}
		matched = true
		e.event.triggerActor = triggerActor
		e.event.triggerActors = triggerActors
		if eff.trigger.kind != penum.SkillTriggerType_SkillTriggerType_Unknown {
			e.event.triggerActors = bound
			e.event.triggerActor = nil
			if len(bound) > 0 {
				e.event.triggerActor = bound[0]
			}
		}
		detail := &papi.LiveSkillDetail{EfficacyIndex: int32(i + 1), StatusType: eff.status, EffectiveChartCount: eff.duration}
		if strings.HasPrefix(eff.operation, "score_get") {
			value := e.skillScore(source, skill, eff, critical)
			detail.Value = value
			if value > 0 && total > int64(^uint64(0)>>1)-value {
				e.err = fmt.Errorf("live skill score overflow")
			} else {
				total += value
			}
		} else if eff.typ == penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectAssignment || eff.typ == penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectAssignmentAll {
			// Transfer is one operation over the complete target set. Executing
			// it per target would consume the source before the second recipient.
			targets := e.targets(eff.target, source)
			e.transferStatuses(source, targets, skill, eff)
			for _, target := range targets {
				detail.TargetCardIndexes = append(detail.TargetCardIndexes, target.index)
				detail.TargetCardInfos = append(detail.TargetCardInfos, &papi.LiveSkillDetailTargetCardInfo{TargetCardIndex: target.index})
			}
		} else {
			handler := effectHandlers[eff.operation]
			if handler == nil {
				e.err = unsupported(eff.id, "no effect executor")
				return nil, 0, false
			}
			seen := make(map[int32]bool, 5)
			targets := e.targets(eff.target, source)
			if eff.operation == "live_ability_cool_time_reduction" {
				targets = nil
				for _, a := range e.actors {
					if a.synthetic && a.side == source.side {
						targets = append(targets, a)
					}
				}
			}
			for _, candidate := range targets {
				target := e.redirectWeakness(source, candidate, eff)
				if target == nil || seen[target.index] {
					continue
				}
				seen[target.index] = true
				value := handler(e, source, target, skill, eff)
				detail.TargetCardIndexes = append(detail.TargetCardIndexes, target.index)
				detail.TargetCardInfos = append(detail.TargetCardInfos, &papi.LiveSkillDetailTargetCardInfo{TargetCardIndex: target.index, Value: value})
				if eff.operation != "status" {
					detail.Value = value
				}
			}
		}
		details = append(details, detail)
	}
	e.event.triggerActor = triggerActor
	e.event.triggerActors = triggerActors
	return details, total, matched
}

func (e *simulation) redirectWeakness(source, target *actor, eff effectSpec) *actor {
	weak := e.isWeakness(eff.status) || eff.operation == "stamina_consumption" || eff.operation == "cool_time_increase" || eff.operation == "strength_effect_value_decrease" || strings.HasPrefix(eff.operation, "strength_effect_erasing")
	if !weak {
		return target
	}
	// Inferred precedence: cover, then one reflection, then timed-debuff
	// prevention. Redirect at most once to avoid reflect/cover recursion cycles.
	if source.side != target.side {
		for _, other := range e.actors {
			if !other.synthetic && other.side == target.side && other != target && e.grade(other, penum.StatusEffectType_StatusEffectType_CoverWeaknessEffect) > 0 {
				target = other
				break
			}
		}
		if e.grade(target, penum.StatusEffectType_StatusEffectType_WeaknessEffectReflection) > 0 {
			target = source
		}
	}
	if eff.operation == "status" && e.isWeakness(eff.status) && e.grade(target, penum.StatusEffectType_StatusEffectType_WeaknessEffectPrevention) > 0 {
		return nil
	}
	return target
}

func applyStatus(e *simulation, source, target *actor, skill *skillState, spec effectSpec) int64 {
	e.addStatus(target, source, skill, spec)
	return 0
}
func (e *simulation) recoveryWeight() int64 {
	v := int64(e.input.Quest.StaminaRecoveryWeightPermil)
	if v == 0 {
		return 1000
	}
	return v
}
func applyRecovery(e *simulation, source, target *actor, skill *skillState, spec effectSpec) int64 {
	value := spec.value
	if spec.operation == "target_stamina_recovery" {
		value = e.scoreValue(float64(target.card.Stamina) * float64(value) / 1000)
	}
	value = e.scoreValue(float64(value) * float64(e.recoveryWeight()) / 1000)
	e.changeStamina(target, value)
	return value
}
func applyDrain(e *simulation, source, target *actor, skill *skillState, spec effectSpec) int64 {
	e.changeStamina(target, -spec.value)
	return spec.value
}

func applyCooldown(e *simulation, source, target *actor, skill *skillState, spec effectSpec) int64 {
	for _, s := range target.skills {
		switch spec.operation {
		case "cool_time_reduction", "live_ability_cool_time_reduction":
			s.cooldown = int32(max(0, int64(s.cooldown)-spec.value))
		case "cool_time_increase":
			if s.cooldown > 0 {
				s.cooldown += int32(spec.value)
			}
		case "passive_skill_cool_time_reset":
			if s.definition.CategoryType == penum.SkillCategoryType_SkillCategoryType_Passive {
				s.cooldown = 0
			}
		}
	}
	target.revision++
	return spec.value
}

func (e *simulation) transferStatuses(source *actor, targets []*actor, skill *skillState, spec effectSpec) {
	if len(targets) == 0 {
		return
	}
	selected := func(s *statusInstance) bool {
		return e.isStrength(s.spec.status) && (spec.typ == penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectAssignmentAll || s.spec.status == spec.secondary)
	}
	var moved []*statusInstance
	for _, s := range source.effects {
		if selected(s) {
			moved = append(moved, s)
		}
	}
	if len(moved) == 0 {
		return
	}
	keepSource := false
	for _, target := range targets {
		if target == source {
			// A self-only transfer remains a no-op. When the source is one
			// of several recipients, retain its original instance just once.
			keepSource = true
			continue
		}
		if len(target.effects)+len(target.migrations)+len(moved) > maxStatusInstances {
			e.err = fmt.Errorf("too many transferred live statuses")
			return
		}
	}
	// Validate every recipient before modifying any actor. A rejected fan-out
	// must not partially consume the source or update earlier recipients.
	for _, target := range targets {
		if target == source {
			continue
		}
		for _, s := range moved {
			// Stages and lifetime already include photo modifiers; preserve
			// them without applying modifiers again. Each recipient needs its
			// own mutable instance for expiry, conversion and migration.
			copy := *s
			copy.source, copy.skill = source.index, skill.ref.Index
			target.effects = append(target.effects, &copy)
		}
		target.revision++
	}
	if !keepSource {
		kept := source.effects[:0]
		for _, s := range source.effects {
			if !selected(s) {
				kept = append(kept, s)
			}
		}
		clear(source.effects[len(kept):])
		source.effects = kept
		source.revision++
	}
}

func applyStatusMutation(e *simulation, source, target *actor, skill *skillState, spec effectSpec) int64 {
	longest := map[penum.StatusEffectType]*statusInstance{}
	if spec.operation == "strength_effect_value_increase" {
		for _, s := range target.effects {
			if e.isStrength(s.spec.status) {
				old := longest[s.spec.status]
				if old == nil || old.remaining < s.remaining {
					longest[s.spec.status] = s
				}
			}
		}
	}
	kept := target.effects[:0]
	for _, s := range target.effects {
		remove := false
		switch spec.operation {
		case "weakness_effect_recovery":
			remove = e.isWeakness(s.spec.status)
		case "weakness_effect_inversion":
			if e.isWeakness(s.spec.status) {
				switch s.spec.status {
				case penum.StatusEffectType_StatusEffectType_DanceDown:
					s.spec.status = penum.StatusEffectType_StatusEffectType_DanceUp
					s.spec.typ = penum.SkillEfficacyType_SkillEfficacyType_DanceUp
				case penum.StatusEffectType_StatusEffectType_VocalDown:
					s.spec.status = penum.StatusEffectType_StatusEffectType_VocalUp
					s.spec.typ = penum.SkillEfficacyType_SkillEfficacyType_VocalUp
				case penum.StatusEffectType_StatusEffectType_VisualDown:
					s.spec.status = penum.StatusEffectType_StatusEffectType_VisualUp
					s.spec.typ = penum.SkillEfficacyType_SkillEfficacyType_VisualUp
				case penum.StatusEffectType_StatusEffectType_StaminaConsumptionIncrease:
					s.spec.status = penum.StatusEffectType_StatusEffectType_StaminaConsumptionReduction
					s.spec.typ = penum.SkillEfficacyType_SkillEfficacyType_StaminaConsumptionReduction
				case penum.StatusEffectType_StatusEffectType_SkillImpossible:
					for _, sk := range target.skills {
						sk.cooldown = max(0, sk.cooldown-s.remaining)
					}
					remove = true
				default:
					remove = true
				}
			}
		case "strength_effect_count_increase":
			if e.isStrength(s.spec.status) {
				s.remaining += int32(spec.value)
			}
		case "strength_effect_value_increase":
			if longest[s.spec.status] == s {
				s.grade += int32(spec.value)
			}
		case "strength_effect_value_decrease":
			if e.isStrength(s.spec.status) {
				s.grade = max(0, s.grade-int32(spec.value))
				remove = s.grade == 0
			}
		case "strength_effect_erasing_all":
			remove = e.isStrength(s.spec.status)
		case "strength_effect_erasing":
			remove = s.spec.status == spec.secondary
		case "status_effect_change":
			if s.spec.status == spec.status {
				s.spec.status = spec.secondary
				s.spec.cap = spec.cap
				s.spec.limitCap = spec.limitCap
				s.spec.typ = statusEfficacy(spec.secondary)
			}
		}
		if !remove {
			kept = append(kept, s)
		}
	}
	clear(target.effects[len(kept):])
	target.effects = kept
	target.revision++
	return spec.value
}

func statusEfficacy(status penum.StatusEffectType) penum.SkillEfficacyType {
	name := strings.TrimPrefix(status.String(), "StatusEffectType_")
	return penum.SkillEfficacyType(penum.SkillEfficacyType_value["SkillEfficacyType_"+name])
}
