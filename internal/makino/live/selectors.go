package live

import (
	"sort"
	"strings"

	"github.com/vertesan/solis-server/internal/proto/penum"
)

func lanePriority(position int) int {
	return position - 1
}

// Official positions are center-first, not left-to-right: 4, 2, 1, 3, 5.
// Target and tie priority still follows the official numeric lane number.
func laneVisualIndex(position int) int {
	switch position {
	case 4:
		return 0
	case 2:
		return 1
	case 1:
		return 2
	case 3:
		return 3
	case 5:
		return 4
	}
	return -1
}

func lanesAreNeighbors(a, b int) bool {
	left, right := laneVisualIndex(a), laneVisualIndex(b)
	return left >= 0 && right >= 0 && (left == right-1 || left == right+1)
}
func actorBefore(a, b *actor) bool {
	if a.side != b.side {
		return a.side < b.side
	}
	return lanePriority(a.position) < lanePriority(b.position)
}
func containsString(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func (e *simulation) coreFanRatio(a *actor) float64 {
	total := a.data.LightFans + a.data.MiddleFans + a.data.HeavyFans
	if total <= 0 {
		return 0
	}
	return float64(a.data.MiddleFans+a.data.HeavyFans) / float64(total)
}
func (e *simulation) hasWeakness(a *actor) bool {
	for _, s := range a.effects {
		if e.isWeakness(s.spec.status) {
			return true
		}
	}
	return false
}
func (e *simulation) isWeakness(status penum.StatusEffectType) bool {
	return e.weaknessTypes[status]
}
func (e *simulation) isStrength(status penum.StatusEffectType) bool {
	return e.strengthTypes[status]
}

func (e *simulation) matches(t triggerSpec, source *actor) (bool, *actor) {
	self := func(ok bool) (bool, *actor) { return ok, source }
	someone := func(predicate func(*actor) bool) (bool, *actor) {
		for _, a := range e.actors {
			if !a.synthetic && a.side == source.side && predicate(a) {
				return true, a
			}
		}
		return false, nil
	}
	combo := int64(e.sides[source.side].combo)
	switch t.kind {
	case penum.SkillTriggerType_SkillTriggerType_Unknown:
		return self(true)
	case penum.SkillTriggerType_SkillTriggerType_Beat:
		return self(e.event.chart != nil)
	case penum.SkillTriggerType_SkillTriggerType_LiveStart:
		return self(e.event.chart == nil)
	case penum.SkillTriggerType_SkillTriggerType_Combo:
		return self(combo >= t.value)
	case penum.SkillTriggerType_SkillTriggerType_MoreThanCharacterCount:
		count := int64(0)
		for _, a := range e.actors {
			if !a.synthetic && a.side == source.side && containsString(t.characters, a.data.CharacterID) {
				count++
			}
		}
		return self(count >= t.value)
	case penum.SkillTriggerType_SkillTriggerType_BeforeActiveSkillBySomeone, penum.SkillTriggerType_SkillTriggerType_BeforeSpecialSkillBySomeone, penum.SkillTriggerType_SkillTriggerType_BeforeSpecialSkill:
		kind := penum.MusicChartType_MusicChartType_ActiveSkill
		if t.kind == penum.SkillTriggerType_SkillTriggerType_BeforeSpecialSkillBySomeone || t.kind == penum.SkillTriggerType_SkillTriggerType_BeforeSpecialSkill {
			kind = penum.MusicChartType_MusicChartType_SpecialSkill
		}
		ok := e.event.chart.GetType() == kind && e.event.winner != nil && e.event.winner.side == source.side && !e.event.activeResolved
		if t.kind == penum.SkillTriggerType_SkillTriggerType_BeforeSpecialSkill {
			ok = ok && e.event.winner == source
		}
		return ok, e.event.winner
	case penum.SkillTriggerType_SkillTriggerType_BeforeCritical:
		return self(e.event.critical[source.index] && !e.event.activeResolved)
	case penum.SkillTriggerType_SkillTriggerType_BeforeCriticalBySomeone:
		return someone(func(a *actor) bool { return e.event.critical[a.index] && !e.event.activeResolved })
	case penum.SkillTriggerType_SkillTriggerType_Critical:
		return self(e.event.critical[source.index] && e.event.activeResolved)
	case penum.SkillTriggerType_SkillTriggerType_Status:
		return self(e.grade(source, t.status) > 0)
	case penum.SkillTriggerType_SkillTriggerType_SomeoneStatus:
		return someone(func(a *actor) bool { return e.grade(a, t.status) > 0 })
	case penum.SkillTriggerType_SkillTriggerType_StaminaLower:
		return self(source.card.Stamina > 0 && source.stamina*100 <= source.card.Stamina*t.value)
	case penum.SkillTriggerType_SkillTriggerType_StaminaHigher:
		return self(source.card.Stamina > 0 && source.stamina*100 >= source.card.Stamina*t.value)
	case penum.SkillTriggerType_SkillTriggerType_SomeoneStaminaLower:
		return someone(func(a *actor) bool { return a.stamina*100 <= a.card.Stamina*t.value })
	case penum.SkillTriggerType_SkillTriggerType_FanEngageHigher:
		return self(e.coreFanRatio(source)*100 >= float64(t.value))
	case penum.SkillTriggerType_SkillTriggerType_Center:
		return self(source.position == 1)
	case penum.SkillTriggerType_SkillTriggerType_MoodType:
		return self(snakeName(strings.TrimPrefix(e.input.Quest.MoodType.String(), "MoodType_")) == t.text)
	case penum.SkillTriggerType_SkillTriggerType_Music:
		return self(e.input.Quest.MusicId == t.text)
	case penum.SkillTriggerType_SkillTriggerType_StatusGroup:
		return self(e.hasWeakness(source))
	case penum.SkillTriggerType_SkillTriggerType_SomeoneStatusGroup:
		return someone(func(a *actor) bool { return e.hasWeakness(a) })
	case penum.SkillTriggerType_SkillTriggerType_PositionAttributeVocal:
		return self(e.laneAttribute(source.position) == penum.AttributeType_AttributeType_Vocal)
	case penum.SkillTriggerType_SkillTriggerType_PositionAttributeDance:
		return self(e.laneAttribute(source.position) == penum.AttributeType_AttributeType_Dance)
	case penum.SkillTriggerType_SkillTriggerType_PositionAttributeVisual:
		return self(e.laneAttribute(source.position) == penum.AttributeType_AttributeType_Visual)
	case penum.SkillTriggerType_SkillTriggerType_SomeoneRecovered:
		return someone(func(a *actor) bool { return e.event.recovered[a.index] })
	case penum.SkillTriggerType_SkillTriggerType_MostLeft:
		return self(source.position == 4)
	case penum.SkillTriggerType_SkillTriggerType_MostRight:
		return self(source.position == 5)
	case penum.SkillTriggerType_SkillTriggerType_ComboLessEqual:
		return self(combo <= t.value)
	case penum.SkillTriggerType_SkillTriggerType_SomeoneStatusEffectGradeHigher:
		return someone(func(a *actor) bool { return int64(e.grade(a, t.status)) >= t.value })
	case penum.SkillTriggerType_SkillTriggerType_AfterActiveSkillByOpponentSomeone:
		for index, category := range e.event.activated {
			a := e.byIndex[index]
			if a != nil && a.side != source.side && category == penum.SkillCategoryType_SkillCategoryType_Active {
				return true, a
			}
		}
		return false, nil
	}
	return false, nil // Compilation rejects every other trigger type.
}

// matchTrigger retains every qualifying actor for target-trigger-N. The scalar
// matcher remains allocation-free for condition checks that only need a bool.
func (e *simulation) matchTrigger(t triggerSpec, source *actor) (bool, []*actor) {
	ok, first := e.matches(t, source)
	if !ok {
		return false, nil
	}
	var predicate func(*actor) bool
	switch t.kind {
	case penum.SkillTriggerType_SkillTriggerType_BeforeCriticalBySomeone:
		predicate = func(a *actor) bool { return e.event.critical[a.index] && !e.event.activeResolved }
	case penum.SkillTriggerType_SkillTriggerType_SomeoneStatus:
		predicate = func(a *actor) bool { return e.grade(a, t.status) > 0 }
	case penum.SkillTriggerType_SkillTriggerType_SomeoneStaminaLower:
		predicate = func(a *actor) bool { return a.stamina*100 <= a.card.Stamina*t.value }
	case penum.SkillTriggerType_SkillTriggerType_SomeoneStatusGroup:
		predicate = e.hasWeakness
	case penum.SkillTriggerType_SkillTriggerType_SomeoneRecovered:
		predicate = func(a *actor) bool { return e.event.recovered[a.index] }
	case penum.SkillTriggerType_SkillTriggerType_SomeoneStatusEffectGradeHigher:
		predicate = func(a *actor) bool { return int64(e.grade(a, t.status)) >= t.value }
	}
	if predicate == nil {
		if first == nil {
			return true, nil
		}
		return true, []*actor{first}
	}
	actors := make([]*actor, 0, 5)
	for _, a := range e.actors {
		if !a.synthetic && a.side == source.side && predicate(a) {
			actors = append(actors, a)
		}
	}
	return len(actors) > 0, actors
}

func (e *simulation) targets(t targetSpec, source *actor) []*actor {
	side := source.side
	if t.opponent {
		side = 1 - side
	}
	var candidates []*actor
	for _, a := range e.actors {
		if a.synthetic || t.kind != penum.SkillTargetType_SkillTargetType_Trigger && a.side != side {
			continue
		}
		ok := true
		switch t.kind {
		case penum.SkillTargetType_SkillTargetType_Self:
			ok = a == source
		case penum.SkillTargetType_SkillTargetType_Center, penum.SkillTargetType_SkillTargetType_OpponentCenter:
			ok = a.position == 1
		case penum.SkillTargetType_SkillTargetType_Status:
			ok = e.grade(a, t.status) > 0
		case penum.SkillTargetType_SkillTargetType_Trigger:
			ok = false
			for _, bound := range e.event.triggerActors {
				if a == bound {
					ok = true
					break
				}
			}
			if len(e.event.triggerActors) == 0 {
				ok = e.event.triggerActor != nil && a == e.event.triggerActor
			}
		case penum.SkillTargetType_SkillTargetType_CardType, penum.SkillTargetType_SkillTargetType_OpponentCardType:
			ok = int64(a.data.Type) == t.value
		case penum.SkillTargetType_SkillTargetType_Neighbor:
			ok = lanesAreNeighbors(a.position, source.position)
		case penum.SkillTargetType_SkillTargetType_Vocal, penum.SkillTargetType_SkillTargetType_OpponentVocal:
			ok = a.data.Attribute == penum.AttributeType_AttributeType_Vocal
		case penum.SkillTargetType_SkillTargetType_Dance, penum.SkillTargetType_SkillTargetType_OpponentDance:
			ok = a.data.Attribute == penum.AttributeType_AttributeType_Dance
		case penum.SkillTargetType_SkillTargetType_Visual, penum.SkillTargetType_SkillTargetType_OpponentVisual:
			ok = a.data.Attribute == penum.AttributeType_AttributeType_Visual
		case penum.SkillTargetType_SkillTargetType_PositionAttributeVocal:
			ok = e.laneAttribute(a.position) == penum.AttributeType_AttributeType_Vocal
		case penum.SkillTargetType_SkillTargetType_PositionAttributeDance, penum.SkillTargetType_SkillTargetType_OpponentPositionAttributeDance:
			ok = e.laneAttribute(a.position) == penum.AttributeType_AttributeType_Dance
		case penum.SkillTargetType_SkillTargetType_PositionAttributeVisual:
			ok = e.laneAttribute(a.position) == penum.AttributeType_AttributeType_Visual
		case penum.SkillTargetType_SkillTargetType_OpponentSamePosition:
			ok = a.position == source.position
		case penum.SkillTargetType_SkillTargetType_StaminaMoreThanEqual:
			ok = a.stamina*100 >= a.card.Stamina*t.value
		case penum.SkillTargetType_SkillTargetType_OpponentStaminaLessThanEqual:
			ok = a.stamina*100 <= a.card.Stamina*t.value
		}
		if ok {
			candidates = append(candidates, a)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return actorBefore(candidates[i], candidates[j]) })
	var value func(*actor) int64
	switch t.kind {
	case penum.SkillTargetType_SkillTargetType_DanceHigher:
		value = func(a *actor) int64 { return e.attribute(a, penum.AttributeType_AttributeType_Dance) }
	case penum.SkillTargetType_SkillTargetType_VocalHigher:
		value = func(a *actor) int64 { return e.attribute(a, penum.AttributeType_AttributeType_Vocal) }
	case penum.SkillTargetType_SkillTargetType_VisualHigher:
		value = func(a *actor) int64 { return e.attribute(a, penum.AttributeType_AttributeType_Visual) }
	case penum.SkillTargetType_SkillTargetType_StaminaHigher:
		value = func(a *actor) int64 { return a.stamina * 1_000_000 / a.card.Stamina }
	case penum.SkillTargetType_SkillTargetType_StaminaLower:
		value = func(a *actor) int64 { return -a.stamina * 1_000_000 / a.card.Stamina }
	}
	if value != nil {
		sort.SliceStable(candidates, func(i, j int) bool { return value(candidates[i]) > value(candidates[j]) })
	}
	if len(candidates) > t.count {
		candidates = candidates[:t.count]
	}
	return candidates
}
