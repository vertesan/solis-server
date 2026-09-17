package live

import (
	"slices"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
)

type activeCandidate struct {
	actor     *actor
	skill     *skillState
	authority float64
}

func (e *simulation) skillFailure(a *actor, s *skillState) penum.SkillFailureType {
	if s.definition.CategoryType == penum.SkillCategoryType_SkillCategoryType_Passive && e.grade(a, penum.StatusEffectType_StatusEffectType_PassiveSkillImpossible) > 0 {
		return penum.SkillFailureType_SkillFailureType_InSkillImpossibleEffect
	}
	if s.definition.CategoryType != penum.SkillCategoryType_SkillCategoryType_Passive && e.grade(a, penum.StatusEffectType_StatusEffectType_SkillImpossible) > 0 {
		return penum.SkillFailureType_SkillFailureType_InSkillImpossibleEffect
	}
	if s.level.LimitCount > 0 && s.remaining <= 0 {
		return penum.SkillFailureType_SkillFailureType_NoRemainingCount
	}
	if s.cooldown > 0 {
		return penum.SkillFailureType_SkillFailureType_InCoolTime
	}
	if !a.synthetic && a.stamina < e.staminaCost(a, s) {
		return penum.SkillFailureType_SkillFailureType_StaminaShortage
	}
	return penum.SkillFailureType_SkillFailureType_Unknown
}
func skillFailureRow(index int32, reason penum.SkillFailureType) *papi.LiveSkillFailure {
	return &papi.LiveSkillFailure{CardIndex: index, Type: reason, EachFailureTypes: []penum.SkillFailureType{reason}}
}
func (e *simulation) candidate(a *actor, category penum.SkillCategoryType) (*skillState, *papi.LiveSkillFailure) {
	f := &papi.LiveSkillFailure{CardIndex: a.index}
	for _, s := range a.skills {
		if s.definition.CategoryType != category {
			continue
		}
		reason := e.skillFailure(a, s)
		if reason == penum.SkillFailureType_SkillFailureType_Unknown {
			return s, nil
		}
		f.EachFailureTypes = append(f.EachFailureTypes, reason)
	}
	if len(f.EachFailureTypes) == 0 {
		return nil, skillFailureRow(a.index, penum.SkillFailureType_SkillFailureType_CategoryMismatch)
	}
	f.Type = f.EachFailureTypes[0]
	return nil, f
}
func (e *simulation) selectActive() []*papi.LiveSkillFailure {
	category := penum.SkillCategoryType_SkillCategoryType_Active
	if e.event.chart.Type == penum.MusicChartType_MusicChartType_SpecialSkill {
		category = penum.SkillCategoryType_SkillCategoryType_Special
	}
	var failures []*papi.LiveSkillFailure
	e.event.candidates = e.event.candidates[:0]
	attribute := e.laneAttribute(int(e.event.chart.Position))
	for side := range e.sides {
		a := e.byIndex[int32(side*5)+e.event.chart.Position]
		if a == nil {
			continue
		}
		// Inspect eligibility without giving a real opportunity to the source or
		// rejected alternatives. Only the final recipient restores its migration.
		preview := e.migrationPreview(a, category)
		if category == penum.SkillCategoryType_SkillCategoryType_Active && e.grade(preview, penum.StatusEffectType_StatusEffectType_ActiveSkillChanceAssignment) > 0 {
			var storage [4]*actor
			replacements := storage[:0]
			for _, other := range e.actors {
				if !other.synthetic && other.side == side && other != a {
					replacements = append(replacements, other)
				}
			}
			// Round6 (notably run075, chart82): compare the attribute required
			// by this chance, not each recipient's own lane or battle authority.
			// Equal values retain ascending official lane order.
			slices.SortStableFunc(replacements, func(left, right *actor) int {
				lv, rv := e.attribute(left, attribute), e.attribute(right, attribute)
				if lv > rv {
					return -1
				}
				if lv < rv {
					return 1
				}
				return left.position - right.position
			})
			for _, other := range replacements {
				if next, _ := e.candidate(e.migrationPreview(other, category), category); next != nil {
					a = other
					break
				}
			}
		}
		// The user's clarified rule treats a transferred anchor as belonging
		// to the recipient all along; the source does not restore. Do not
		// recursively transfer a recipient's chance if it has assignment too.
		e.restoreMigratedStatuses(a, category)
		s, f := e.candidate(a, category)
		if s == nil {
			failures = append(failures, f)
			continue
		}
		// Migration follows the recipient, but the opportunity retains its
		// original required attribute for scoring and battle authority.
		e.event.candidates = append(e.event.candidates, activeCandidate{actor: a, skill: s, authority: e.authority(a, attribute)})
	}
	// Freeze authority before early passives. Exact ties retain the attacking side.
	if len(e.event.candidates) == 2 && e.event.candidates[1].authority > e.event.candidates[0].authority {
		e.event.candidates[0], e.event.candidates[1] = e.event.candidates[1], e.event.candidates[0]
	}
	if len(e.event.candidates) > 0 {
		e.event.winner = e.event.candidates[0].actor
		e.event.winningSkill = e.event.candidates[0].skill
	}
	return failures
}

func (e *simulation) breakCombo(side int) {
	for _, a := range e.actors {
		if !a.synthetic && a.side == side && e.grade(a, penum.StatusEffectType_StatusEffectType_ComboContinuation) > 0 {
			return
		}
	}
	e.sides[side].combo = 0
}
func (e *simulation) activeSkill(failures []*papi.LiveSkillFailure) *papi.LiveSkill {
	var attempted *papi.LiveSkill
	var failedDescriptions []*papi.LiveLogDescription
	for i, candidate := range e.event.candidates {
		a, s := candidate.actor, candidate.skill
		e.event.winner, e.event.winningSkill = a, s
		e.event.critical[a.index] = e.event.criticalRolls[a.index]
		if reason := e.skillFailure(a, s); reason != penum.SkillFailureType_SkillFailureType_Unknown {
			failures = append(failures, skillFailureRow(a.index, reason))
			e.breakCombo(a.side)
			e.event.critical[a.index] = false
			continue
		}
		e.event.triggerActor = a
		e.event.triggerActors = []*actor{a}
		out := e.activate(a, s, true, e.event.critical[a.index])
		attempted = out
		if !out.Activated {
			failures = append(failures, out.Failures...)
			failedDescriptions = append(failedDescriptions, out.Descriptions...)
			e.breakCombo(a.side)
			e.event.critical[a.index] = false
			continue
		}
		// A lower-authority opposing candidate may activate if the first attempt
		// fails. Losing authority itself spends no stamina, cooldown or uses.
		for _, loser := range e.event.candidates[i+1:] {
			failures = append(failures, skillFailureRow(loser.actor.index, penum.SkillFailureType_SkillFailureType_OpponentActivation))
		}
		out.Failures = append(out.Failures, failures...)
		out.Descriptions = append(e.failureDescriptions(failures), out.Descriptions...)
		e.incrementCombo(e.sides[a.side])
		e.sides[a.side].info.UserResult.HitCount++
		if out.IsCritical {
			e.sides[a.side].info.UserResult.CriticalCount++
		}
		out.CardStatuses = e.cardSnapshots()
		out.UserStatuses = e.userSnapshots()
		return out
	}
	out := &papi.LiveSkill{Order: e.nextOrder(), Failures: failures, IsComboBreak: true, Descriptions: failedDescriptions}
	// Single-side misses keep their attempted actor for the failure animation;
	// a battle with no winner uses the established index-zero sentinel.
	if len(e.sides) == 1 && attempted != nil {
		out.CardIndex = attempted.CardIndex
		out.SkillIndex = attempted.SkillIndex
	}
	for side := range e.sides {
		e.breakCombo(side)
	}
	e.event.winner, e.event.winningSkill = nil, nil
	if len(out.Descriptions) == 0 {
		out.Descriptions = e.failureDescriptions(failures)
	}
	out.CardStatuses = e.cardSnapshots()
	out.UserStatuses = e.userSnapshots()
	return out
}
func cooldownBonusForSlot(slot int32) penum.PhotoAbilityType {
	switch slot {
	case 1:
		return penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill1
	case 2:
		return penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill2
	case 3:
		return penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill3
	default:
		return penum.PhotoAbilityType_PhotoAbilityType_Unknown
	}
}
func (e *simulation) activate(a *actor, s *skillState, active, critical bool) *papi.LiveSkill {
	out := &papi.LiveSkill{Order: e.nextOrder(), CardIndex: a.index, SkillIndex: s.ref.Index}
	s.usedEvent = e.event.ordinal
	s.currentStaminaCost = 0
	if !active && e.rng.IntN(1000) >= int(s.level.ProbabilityPermil) {
		out.Descriptions = e.skillDescriptions(a, s, out)
		return out
	}
	// User-confirmed: failed skills consume no stamina, cooldown or uses.
	if active && e.rng.Float64() >= min(1, max(0, e.successRate(a))) {
		out.Failures = []*papi.LiveSkillFailure{skillFailureRow(a.index, penum.SkillFailureType_SkillFailureType_Miss)}
		e.sides[a.side].info.UserResult.MissCount++
		out.Descriptions = e.skillDescriptions(a, s, out)
		return out
	}
	beforeStamina, beforeConsumed := a.stamina, a.consumed
	beforeCooldown, beforeRemaining := s.cooldown, s.remaining
	cost := int64(0)
	if !a.synthetic {
		cost = e.staminaCost(a, s)
		e.changeStamina(a, -cost)
	}
	s.currentStaminaCost = cost
	// Successful self-target cooldown effects must see this activation's new
	// cooldown. These provisional resources are restored if no detail executes.
	if s.level.LimitCount > 0 {
		s.remaining--
	}
	ctReduction := a.data.Bonuses[cooldownBonusForSlot(s.ref.Index)]
	s.cooldown = int32(max(0, int64(s.level.CoolTime)-ctReduction))
	a.revision++
	var matched bool
	out.Details, out.Score, matched = e.executeEffects(a, s, critical)
	if !matched {
		// Detail predicates observe post-cost stamina. No effects were applied if
		// every predicate failed, so undo only this provisional stamina debit.
		a.stamina, a.consumed = beforeStamina, beforeConsumed
		s.cooldown, s.remaining = beforeCooldown, beforeRemaining
		a.revision++
		s.currentStaminaCost = 0
		out.Failures = []*papi.LiveSkillFailure{skillFailureRow(a.index, penum.SkillFailureType_SkillFailureType_AllSkillEfficacyTriggerFail)}
		out.Descriptions = e.skillDescriptions(a, s, out)
		return out
	}
	out.Stamina = cost
	out.Activated = true
	out.IsCritical = critical
	a.activations++
	if active {
		e.event.activated[a.index] = s.definition.CategoryType
	}
	if out.Score > 0 {
		e.addScore(a, out.Score)
	}
	out.Descriptions = e.skillDescriptions(a, s, out)
	return out
}
func isBeforeTrigger(kind penum.SkillTriggerType) bool {
	switch kind {
	case penum.SkillTriggerType_SkillTriggerType_BeforeActiveSkillBySomeone,
		penum.SkillTriggerType_SkillTriggerType_BeforeSpecialSkillBySomeone,
		penum.SkillTriggerType_SkillTriggerType_BeforeCritical,
		penum.SkillTriggerType_SkillTriggerType_BeforeCriticalBySomeone,
		penum.SkillTriggerType_SkillTriggerType_BeforeSpecialSkill:
		return true
	default:
		return false
	}
}
func (e *simulation) passives(phase string) []*papi.LiveSkill {
	var out []*papi.LiveSkill
	for _, a := range e.passiveActors {
		for _, s := range a.skills {
			if s.definition.CategoryType != penum.SkillCategoryType_SkillCategoryType_Passive || s.usedEvent == e.event.ordinal {
				continue
			}
			before := isBeforeTrigger(s.trigger.kind)
			if phase == "start" && s.trigger.kind != penum.SkillTriggerType_SkillTriggerType_LiveStart {
				continue
			}
			if phase != "start" && s.trigger.kind == penum.SkillTriggerType_SkillTriggerType_LiveStart {
				continue
			}
			if phase == "early" && s.trigger.kind != penum.SkillTriggerType_SkillTriggerType_Unknown && !before {
				continue
			}
			if phase == "late" && before {
				continue
			}
			quota := int64(a.index)*10000 + int64(s.ref.PossessionType)
			if a.synthetic {
				quota = int64(a.index)*10000 + int64(s.ref.Index)
			}
			if e.event.budget[quota] {
				continue
			}
			ok, _ := e.matches(s.trigger, a)
			reason := e.skillFailure(a, s)
			if !ok || reason != penum.SkillFailureType_SkillFailureType_Unknown {
				if phase == "late" || before || phase == "start" {
					row := &papi.LiveSkill{Order: e.nextOrder(), CardIndex: a.index, SkillIndex: s.ref.Index}
					if ok {
						row.Descriptions = e.failureDescriptions([]*papi.LiveSkillFailure{skillFailureRow(a.index, reason)})
					} else {
						row.Descriptions = []*papi.LiveLogDescription{{Message: e.skillLabel(a, s) + ": 発動条件未成立（消費なし）"}}
					}
					out = append(out, row)
					s.usedEvent = e.event.ordinal
				}
				continue
			}
			_, e.event.triggerActors = e.matchTrigger(s.trigger, a)
			e.event.triggerActor = nil
			if len(e.event.triggerActors) > 0 {
				e.event.triggerActor = e.event.triggerActors[0]
			}
			skill := e.activate(a, s, false, false)
			if skill.Activated {
				e.event.budget[quota] = true
				skill.CardStatuses = e.cardSnapshots()
				skill.UserStatuses = e.userSnapshots()
			}
			out = append(out, skill)
		}
	}
	return out
}
