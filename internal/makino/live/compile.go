package live

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

// UnsupportedError identifies the exact definition that must be implemented.
// Adapters must not consume resources when compilation fails.
type UnsupportedError struct{ Definition, Reason string }

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("unsupported live definition %q: %s", e.Definition, e.Reason)
}
func unsupported(id, reason string) error { return &UnsupportedError{Definition: id, Reason: reason} }

type triggerSpec struct {
	id         string
	kind       penum.SkillTriggerType
	value      int64
	status     penum.StatusEffectType
	characters []string
	text       string
}
type targetSpec struct {
	id       string
	kind     penum.SkillTargetType
	opponent bool
	count    int
	value    int64
	status   penum.StatusEffectType
}
type effectSpec struct {
	id, operation                  string
	typ                            penum.SkillEfficacyType
	status, secondary              penum.StatusEffectType
	grade, cap, limitCap, duration int32
	value                          int64
	target                         targetSpec
	trigger                        triggerSpec
	conditional                    triggerSpec
	conditionalPower               int64
	text                           string
}
type skillState struct {
	ref                 *papi.LiveDeckCardSkill
	definition          *pmaster.Skill
	level               *pmaster.SkillLevel
	trigger             triggerSpec
	effects             []effectSpec
	cooldown, remaining int32
	usedEvent           int
	currentStaminaCost  int64
}

func snakeName(name string) string {
	var out strings.Builder
	for i, r := range name {
		if unicode.IsUpper(r) && i > 0 {
			out.WriteByte('_')
		}
		out.WriteRune(unicode.ToLower(r))
	}
	return out.String()
}

var statusNames = func() map[string]penum.StatusEffectType {
	out := make(map[string]penum.StatusEffectType)
	for value, name := range penum.StatusEffectType_name {
		out[snakeName(strings.TrimPrefix(name, "StatusEffectType_"))] = penum.StatusEffectType(value)
	}
	// Old target IDs use this generic label for A-skill score up.
	out["skill_score_up"] = penum.StatusEffectType_StatusEffectType_ActiveSkillScoreUp
	out["vocal"] = penum.StatusEffectType_StatusEffectType_VocalUp
	return out
}()

func statusID(id string) (penum.StatusEffectType, error) {
	value, ok := statusNames[strings.TrimSpace(id)]
	if !ok || value == penum.StatusEffectType_StatusEffectType_Unknown {
		return penum.StatusEffectType_StatusEffectType_Unknown, unsupported(id, "unknown status name")
	}
	return value, nil
}

func nonnegativeInt(id, raw string) (int64, error) {
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || v < 0 {
		return 0, unsupported(id, "invalid integer parameter "+raw)
	}
	return v, nil
}

func compileTrigger(in *Input, id string) (triggerSpec, error) {
	t := triggerSpec{id: id}
	if id == "" {
		return t, nil
	}
	m := in.Triggers[id]
	if m == nil {
		return t, unsupported(id, "missing trigger master")
	}
	t.kind = m.Type
	t.characters = m.CharacterIds
	raw := strings.TrimPrefix(id, "tg-")
	parts := strings.Split(raw, "-")
	lastNumber := func() error { var err error; t.value, err = nonnegativeInt(id, parts[len(parts)-1]); return err }
	switch t.kind {
	case penum.SkillTriggerType_SkillTriggerType_Beat, penum.SkillTriggerType_SkillTriggerType_LiveStart, penum.SkillTriggerType_SkillTriggerType_BeforeActiveSkillBySomeone, penum.SkillTriggerType_SkillTriggerType_BeforeSpecialSkillBySomeone, penum.SkillTriggerType_SkillTriggerType_BeforeCritical, penum.SkillTriggerType_SkillTriggerType_BeforeCriticalBySomeone, penum.SkillTriggerType_SkillTriggerType_Critical, penum.SkillTriggerType_SkillTriggerType_Center, penum.SkillTriggerType_SkillTriggerType_StatusGroup, penum.SkillTriggerType_SkillTriggerType_SomeoneStatusGroup, penum.SkillTriggerType_SkillTriggerType_PositionAttributeVocal, penum.SkillTriggerType_SkillTriggerType_PositionAttributeDance, penum.SkillTriggerType_SkillTriggerType_PositionAttributeVisual, penum.SkillTriggerType_SkillTriggerType_SomeoneRecovered, penum.SkillTriggerType_SkillTriggerType_MostLeft, penum.SkillTriggerType_SkillTriggerType_MostRight, penum.SkillTriggerType_SkillTriggerType_BeforeSpecialSkill, penum.SkillTriggerType_SkillTriggerType_AfterActiveSkillByOpponentSomeone:
	case penum.SkillTriggerType_SkillTriggerType_Combo, penum.SkillTriggerType_SkillTriggerType_MoreThanCharacterCount, penum.SkillTriggerType_SkillTriggerType_StaminaLower, penum.SkillTriggerType_SkillTriggerType_StaminaHigher, penum.SkillTriggerType_SkillTriggerType_SomeoneStaminaLower, penum.SkillTriggerType_SkillTriggerType_FanEngageHigher, penum.SkillTriggerType_SkillTriggerType_ComboLessEqual:
		if err := lastNumber(); err != nil {
			return t, err
		}
		if t.kind == penum.SkillTriggerType_SkillTriggerType_MoreThanCharacterCount && len(t.characters) == 0 {
			return t, unsupported(id, "character-count trigger has no character set")
		}
	case penum.SkillTriggerType_SkillTriggerType_Status, penum.SkillTriggerType_SkillTriggerType_SomeoneStatus:
		if len(parts) != 2 {
			return t, unsupported(id, "status trigger shape")
		}
		var err error
		t.status, err = statusID(parts[1])
		if err != nil {
			return t, err
		}
	case penum.SkillTriggerType_SkillTriggerType_MoodType:
		t.text = strings.TrimPrefix(raw, "mood_type-")
		found := false
		for _, name := range penum.MoodType_name {
			if snakeName(strings.TrimPrefix(name, "MoodType_")) == t.text {
				found = true
			}
		}
		if !found {
			return t, unsupported(id, "unknown mood")
		}
	case penum.SkillTriggerType_SkillTriggerType_Music:
		t.text = strings.TrimPrefix(raw, "music-")
	case penum.SkillTriggerType_SkillTriggerType_SomeoneStatusEffectGradeHigher:
		if err := lastNumber(); err != nil {
			return t, err
		}
		name := strings.TrimSuffix(strings.TrimPrefix(raw, "someone_status_effect_grade_higher_"), "-"+parts[len(parts)-1])
		var err error
		t.status, err = statusID(name)
		if err != nil {
			return t, err
		}
	default:
		return t, unsupported(id, "trigger type "+m.Type.String())
	}
	return t, nil
}

func compileTarget(in *Input, id string) (targetSpec, error) {
	t := targetSpec{id: id, count: 5}
	if id == "" {
		t.kind = penum.SkillTargetType_SkillTargetType_Self
		t.count = 1
		return t, nil
	}
	m := in.Targets[id]
	if m == nil {
		return t, unsupported(id, "missing target master")
	}
	t.kind = m.Type
	t.opponent = m.IsOpponent
	parts := strings.Split(id, "-")
	last := func() error { v, e := nonnegativeInt(id, parts[len(parts)-1]); t.count = int(v); return e }
	switch t.kind {
	case penum.SkillTargetType_SkillTargetType_Self, penum.SkillTargetType_SkillTargetType_Center, penum.SkillTargetType_SkillTargetType_Trigger, penum.SkillTargetType_SkillTargetType_OpponentCenter, penum.SkillTargetType_SkillTargetType_OpponentSamePosition:
		t.count = 1
		if t.kind == penum.SkillTargetType_SkillTargetType_Trigger && len(parts) > 2 {
			if err := last(); err != nil {
				return t, err
			}
		}
	case penum.SkillTargetType_SkillTargetType_All, penum.SkillTargetType_SkillTargetType_Neighbor, penum.SkillTargetType_SkillTargetType_OpponentAll:
	case penum.SkillTargetType_SkillTargetType_DanceHigher, penum.SkillTargetType_SkillTargetType_VocalHigher, penum.SkillTargetType_SkillTargetType_VisualHigher, penum.SkillTargetType_SkillTargetType_StaminaHigher, penum.SkillTargetType_SkillTargetType_StaminaLower, penum.SkillTargetType_SkillTargetType_Vocal, penum.SkillTargetType_SkillTargetType_Dance, penum.SkillTargetType_SkillTargetType_Visual, penum.SkillTargetType_SkillTargetType_PositionAttributeVocal, penum.SkillTargetType_SkillTargetType_PositionAttributeDance, penum.SkillTargetType_SkillTargetType_PositionAttributeVisual, penum.SkillTargetType_SkillTargetType_OpponentVocal, penum.SkillTargetType_SkillTargetType_OpponentDance, penum.SkillTargetType_SkillTargetType_OpponentVisual, penum.SkillTargetType_SkillTargetType_OpponentPositionAttributeDance:
		if err := last(); err != nil {
			return t, err
		}
	case penum.SkillTargetType_SkillTargetType_Status:
		if len(parts) != 4 {
			return t, unsupported(id, "status target shape")
		}
		var err error
		t.status, err = statusID(parts[2])
		if err != nil {
			return t, err
		}
		if err = last(); err != nil {
			return t, err
		}
	case penum.SkillTargetType_SkillTargetType_CardType, penum.SkillTargetType_SkillTargetType_OpponentCardType, penum.SkillTargetType_SkillTargetType_StaminaMoreThanEqual, penum.SkillTargetType_SkillTargetType_OpponentStaminaLessThanEqual:
		if len(parts) != 4 {
			return t, unsupported(id, "parameterized target shape")
		}
		v, err := nonnegativeInt(id, parts[2])
		if err != nil {
			return t, err
		}
		w, err := nonnegativeInt(id, parts[3])
		if err != nil {
			return t, err
		}
		if t.kind == penum.SkillTargetType_SkillTargetType_CardType || t.kind == penum.SkillTargetType_SkillTargetType_OpponentCardType {
			t.value = v
			t.count = int(w)
		} else {
			t.count = int(v)
			t.value = w
		}
	default:
		return t, unsupported(id, "target type "+m.Type.String())
	}
	if t.count < 1 || t.count > 5 {
		return t, unsupported(id, "target count outside unit size")
	}
	return t, nil
}

func compileEffect(in *Input, id, triggerID string) (effectSpec, error) {
	e := effectSpec{id: id}
	m := in.Efficacies[id]
	if m == nil {
		return e, unsupported(id, "missing efficacy master")
	}
	e.typ = m.Type
	e.grade = m.Grade
	e.cap = m.MaxGrade
	e.limitCap = m.LimitBreakMaxGrade
	var err error
	e.trigger, err = compileTrigger(in, triggerID)
	if err != nil {
		return e, err
	}
	e.target, err = compileTarget(in, m.SkillTargetId)
	if err != nil {
		return e, err
	}
	body := strings.TrimPrefix(id, "ef-")
	parts := strings.Split(body, "-")
	e.operation = parts[0]
	if s, ok := statusNames[e.operation]; ok && s != penum.StatusEffectType_StatusEffectType_Unknown {
		e.status = s
		if m.SkillTargetId == "" {
			return e, unsupported(id, "status effect without target")
		}
		targetParts := strings.SplitN(body, m.SkillTargetId, 2)
		if len(targetParts) != 2 {
			return e, unsupported(id, "target ID absent from efficacy encoding")
		}
		duration := strings.TrimPrefix(targetParts[1], "-")
		v, err := nonnegativeInt(id, duration)
		if err != nil {
			return e, err
		}
		e.duration = int32(v)
		if e.duration < 1 {
			return e, unsupported(id, "nonpositive status duration")
		}
		if e.grade == 0 {
			e.grade = 1
		}
		if e.cap == 0 {
			e.cap = e.grade
		}
		e.operation = "status"
		return e, nil
	}
	switch e.operation {
	case "score_get", "score_get_by_less_fan_amount", "score_get_by_more_fan_engage", "score_get_by_more_stamina", "score_get_by_less_stamina", "score_get_by_more_combo_count", "score_get_by_strength_effect_count", "score_get_by_less_combo_count", "score_get_by_skill_activation_count", "score_get_by_more_stamina_use", "score_get_by_skill_success_rate_up", "score_get_and_stamina_consumption_by_more_stamina_use":
		if len(parts) != 3 || parts[2] != "chart_dependence" {
			return e, unsupported(id, "score parameter shape")
		}
		e.value, err = nonnegativeInt(id, parts[1])
	case "score_get_by_status_effect_type_grade", "score_get_by_character_count":
		if len(parts) != 4 || parts[3] != "chart_dependence" {
			return e, unsupported(id, "conditional score shape")
		}
		e.text = parts[1]
		e.value, err = nonnegativeInt(id, parts[2])
		if e.operation == "score_get_by_status_effect_type_grade" && err == nil {
			e.secondary, err = statusID(e.text)
			if err == nil {
				if _, ok := conditionalStatusCoefficient(e.secondary); !ok {
					return e, unsupported(id, "conditional status power coefficient is unknown")
				}
			}
		}
	case "score_get_by_trigger":
		// ef-score_get_by_trigger-BASE-tg-...-BOOST-chart_dependence.
		if len(parts) < 6 || parts[len(parts)-1] != "chart_dependence" {
			return e, unsupported(id, "trigger score shape")
		}
		e.value, err = nonnegativeInt(id, parts[1])
		if err != nil {
			return e, err
		}
		e.conditionalPower, err = nonnegativeInt(id, parts[len(parts)-2])
		if err != nil {
			return e, err
		}
		e.conditional, err = compileTrigger(in, strings.Join(parts[2:len(parts)-2], "-"))
	case "fix_stamina_recovery", "target_stamina_recovery", "stamina_consumption", "strength_effect_count_increase", "strength_effect_value_increase", "strength_effect_value_decrease", "cool_time_reduction", "cool_time_increase", "score_get_by_score_ratio":
		if len(parts) < 3 {
			return e, unsupported(id, "effect value missing")
		}
		e.value, err = nonnegativeInt(id, parts[1])
	case "live_ability_cool_time_reduction":
		if len(parts) != 2 {
			return e, unsupported(id, "live bonus cooldown shape")
		}
		e.value, err = nonnegativeInt(id, parts[1])
	case "weakness_effect_recovery", "weakness_effect_inversion", "strength_effect_erasing_all", "strength_effect_assignment_all", "passive_skill_cool_time_reset":
	case "strength_effect_migration_before_active_skill", "strength_effect_migration_before_special_skill":
		if m.SkillTargetId == "" {
			return e, unsupported(id, "migration effect without target")
		}
	case "strength_effect_erasing", "strength_effect_assignment":
		if len(parts) < 3 {
			return e, unsupported(id, "effect status missing")
		}
		e.secondary, err = statusID(parts[1])
	case "status_effect_change":
		if m.SkillTargetId == "" {
			return e, unsupported(id, "change effect without target")
		}
		targetParts := strings.SplitN(body, m.SkillTargetId, 2)
		if len(targetParts) != 2 {
			return e, unsupported(id, "target ID absent from change encoding")
		}
		rest := strings.Split(strings.TrimPrefix(targetParts[1], "-"), "-")
		if len(rest) != 2 {
			return e, unsupported(id, "status change shape")
		}
		e.status, err = statusID(rest[0])
		if err != nil {
			return e, err
		}
		e.secondary, err = statusID(rest[1])
		if err == nil {
			// The conversion definition has no caps. They belong to the
			// destination status, whose normal efficacy is in the master set.
			destination := statusEfficacy(e.secondary)
			for _, row := range in.Efficacies {
				if row.Type == destination {
					e.cap = max(e.cap, row.MaxGrade)
					e.limitCap = max(e.limitCap, row.LimitBreakMaxGrade)
				}
			}
			if e.cap == 0 {
				return e, unsupported(id, "missing destination status caps")
			}
		}
	case "score_get_by_strength_effect_count_limit_increase":
		e.operation = "status"
		e.status = penum.StatusEffectType_StatusEffectType_ScoreGetByStrengthEffectCountLimitUp
		if len(parts) < 5 {
			return e, unsupported(id, "buff-kind cap shape")
		}
		e.value, err = nonnegativeInt(id, parts[1])
		if err != nil {
			return e, err
		}
		e.grade = int32(e.value)
		e.cap = e.grade
		v, er := nonnegativeInt(id, parts[len(parts)-1])
		err = er
		e.duration = int32(v)
	default:
		return e, unsupported(id, "efficacy family "+e.operation)
	}
	return e, err
}

// ConversionStatusEfficacyType lets adapters resolve destination status caps
// without duplicating the encoded master-ID parser or loading all efficacies.
func ConversionStatusEfficacyType(row *pmaster.SkillEfficacy) (penum.SkillEfficacyType, bool, error) {
	if row == nil || row.Type != penum.SkillEfficacyType_SkillEfficacyType_StatusEffectChange {
		return penum.SkillEfficacyType_SkillEfficacyType_Unknown, false, nil
	}
	prefix := "ef-status_effect_change-" + row.SkillTargetId + "-"
	if row.SkillTargetId == "" || !strings.HasPrefix(row.Id, prefix) {
		return penum.SkillEfficacyType_SkillEfficacyType_Unknown, true, unsupported(row.Id, "status change shape")
	}
	parts := strings.Split(strings.TrimPrefix(row.Id, prefix), "-")
	if len(parts) != 2 {
		return penum.SkillEfficacyType_SkillEfficacyType_Unknown, true, unsupported(row.Id, "status change shape")
	}
	status, err := statusID(parts[1])
	if err != nil {
		return penum.SkillEfficacyType_SkillEfficacyType_Unknown, true, err
	}
	typ := statusEfficacy(status)
	if typ == penum.SkillEfficacyType_SkillEfficacyType_Unknown {
		return typ, true, unsupported(row.Id, "unknown destination efficacy")
	}
	return typ, true, nil
}

func compileSkill(in *Input, ref *papi.LiveDeckCardSkill) (*skillState, error) {
	if ref == nil || ref.Index < 1 || ref.SkillLevel < 1 {
		return nil, unsupported("skill", "invalid learned skill reference")
	}
	m := in.Skills[ref.SkillId]
	if m == nil {
		return nil, unsupported(ref.SkillId, "missing skill master")
	}
	if m.CategoryType != penum.SkillCategoryType_SkillCategoryType_Special && m.CategoryType != penum.SkillCategoryType_SkillCategoryType_Active && m.CategoryType != penum.SkillCategoryType_SkillCategoryType_Passive {
		return nil, unsupported(ref.SkillId, "unknown skill category")
	}
	var level *pmaster.SkillLevel
	for _, row := range m.Levels {
		if row.Level == ref.SkillLevel {
			level = row
			break
		}
	}
	if level == nil {
		return nil, unsupported(ref.SkillId, "missing learned level")
	}
	if level.Stamina < 0 || level.StaminaPermil < 0 || level.CoolTime < 0 || level.LimitCount < 0 || level.ProbabilityPermil < 0 || level.ProbabilityPermil > 1000 {
		return nil, unsupported(ref.SkillId, "invalid skill costs or probability")
	}
	trigger, err := compileTrigger(in, level.TriggerId)
	if err != nil {
		return nil, err
	}
	s := &skillState{ref: ref, definition: m, level: level, trigger: trigger, remaining: level.LimitCount, usedEvent: -1}
	for i, detail := range level.SkillDetails {
		if detail == nil {
			return nil, unsupported(ref.SkillId, "nil skill detail")
		}
		id := detail.EfficacyId
		if i < len(ref.RewrittenEfficacyIds) && ref.RewrittenEfficacyIds[i] != "" {
			id = ref.RewrittenEfficacyIds[i]
		}
		e, err := compileEffect(in, id, detail.TriggerId)
		if err != nil {
			return nil, err
		}
		s.effects = append(s.effects, e)
	}
	if len(s.effects) == 0 {
		return nil, unsupported(ref.SkillId, "skill has no effects")
	}
	return s, nil
}
