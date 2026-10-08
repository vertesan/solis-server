package live

import (
	"math"
	"sort"

	"github.com/vertesan/solis-server/internal/proto/penum"
)

// Authored low-fan catch-up curve; neither coefficient is an official datum.
const lowFanPowerMaximumBonus = 0.8
const lowFanTargetCapacityMultiplier = 2

// These score equations follow the user's confirmed composition. RNG ordering,
// attention redistribution and unverified conditional coefficients are private
// server rules, not claims of bit-for-bit official-server reproduction.
func (e *simulation) beatScore(a *actor, critical bool) int64 {
	q := e.input.Quest
	// Both Exercise and ordinary captures fit a song-level coefficient. Floor
	// each weighted attribute separately; capture060/chart203 distinguishes
	// this from flooring their sum. Resolve the coefficient once, not per beat.
	weighted := math.Floor(float64(e.attribute(a, penum.AttributeType_AttributeType_Vocal))*float64(q.GetBeatVocalWeightPermil())/1000) +
		math.Floor(float64(e.attribute(a, penum.AttributeType_AttributeType_Dance))*float64(q.GetBeatDanceWeightPermil())/1000) +
		math.Floor(float64(e.attribute(a, penum.AttributeType_AttributeType_Visual))*float64(q.GetBeatVisualWeightPermil())/1000)
	base := weighted * float64(e.beatPermil) / 1000
	bonus := 1 + float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_ScoreUp)+e.effectValue(a, penum.StatusEffectType_StatusEffectType_BeatScoreUp)+e.effectValue(a, penum.StatusEffectType_StatusEffectType_BeatBoost)+a.data.Bonuses[penum.PhotoAbilityType_PhotoAbilityType_BeatScoreMultiply])/1000
	value := base*bonus*e.comboFactor(a)*e.fanFactor(a)*e.criticalFactor(a, critical)*characterScoreFactor(a)*e.jitter() + float64(a.data.Bonuses[penum.PhotoAbilityType_PhotoAbilityType_BeatScoreAdd])
	return e.scoreValue(value)
}

func (e *simulation) skillScore(a *actor, s *skillState, eff effectSpec, critical bool) int64 {
	power := e.skillPower(a, s, eff)
	if e.err != nil {
		return 0
	}
	category := s.definition.GetCategoryType()
	stage, bonus, fixed := 1.0, 1+float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_ScoreUp)+e.effectValue(a, penum.StatusEffectType_StatusEffectType_TensionUp))/1000, 0.0
	switch category {
	case penum.SkillCategoryType_SkillCategoryType_Active:
		stage = float64(e.input.Quest.GetActiveSkillWeightPermil()) / 1000
		bonus += float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_ActiveSkillScoreUp)+a.data.Bonuses[penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreMultiply]) / 1000
		fixed = float64(a.data.Bonuses[penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreAdd])
		power += float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_ActiveScoreMultiplierAdd)) / 1000
	case penum.SkillCategoryType_SkillCategoryType_Special:
		stage = float64(e.input.Quest.GetSpecialSkillWeightPermil()) / 1000
		bonus += float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_SpecialSkillScoreUp)+a.data.Bonuses[penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreMultiply]) / 1000
		fixed = float64(a.data.Bonuses[penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreAdd])
		power += float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_SpecialScoreMultiplierAdd)) / 1000
	case penum.SkillCategoryType_SkillCategoryType_Passive:
		// Current official help includes tension for P skills, unlike the older workbook.
		bonus += float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_PassiveSkillScoreUp)) / 1000
		power += float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_PassiveScoreMultiplierAdd)) / 1000
	}
	attribute := e.laneAttribute(a.position)
	if category != penum.SkillCategoryType_SkillCategoryType_Passive && e.event.chart != nil {
		// Reassignment changes who owns the opportunity, not its attribute.
		// P skills still use their caster's own lane attribute.
		attribute = e.laneAttribute(int(e.event.chart.Position))
	}
	base := float64(e.attribute(a, attribute))
	audienceCombo := e.comboFactor(a) * e.fanFactor(a)
	if eff.operation == "score_get_by_score_ratio" {
		base = 0
		for _, target := range e.targets(eff.target, a) {
			base += float64(target.score)
		}
		// The workbook explicitly excludes both multipliers from cumulative-score skills.
		audienceCombo = 1
	}
	return e.scoreValue(base*power*bonus*audienceCombo*stage*e.criticalFactor(a, critical)*characterScoreFactor(a)*e.jitter() + fixed)
}

func characterScoreFactor(a *actor) float64 {
	// An omitted adapter value is neutral, including synthetic stage actors.
	if a.data.CharacterAdvantagePermil == 0 {
		return 1
	}
	return float64(a.data.CharacterAdvantagePermil) / 1000
}

func conditionalStatusCoefficient(status penum.StatusEffectType) (float64, bool) {
	switch status {
	case penum.StatusEffectType_StatusEffectType_TensionUp:
		return .16, true
	case penum.StatusEffectType_StatusEffectType_CriticalRateUp:
		return .067, true
	case penum.StatusEffectType_StatusEffectType_ActiveSkillScoreUp, penum.StatusEffectType_StatusEffectType_CriticalBonusPermilUp:
		return .063, true
	case penum.StatusEffectType_StatusEffectType_AudienceAmountIncrease:
		return .07, true
	case penum.StatusEffectType_StatusEffectType_DanceUp, penum.StatusEffectType_StatusEffectType_VocalUp, penum.StatusEffectType_StatusEffectType_VisualUp, penum.StatusEffectType_StatusEffectType_StaminaConsumptionReduction, penum.StatusEffectType_StatusEffectType_VocalBoost, penum.StatusEffectType_StatusEffectType_DanceBoost, penum.StatusEffectType_StatusEffectType_VisualBoost:
		return .06, true
	default:
		return 0, false
	}
}

func (e *simulation) skillPower(a *actor, s *skillState, eff effectSpec) float64 {
	factor := 1.0
	ratio := 0.0
	if a.card.Stamina > 0 {
		ratio = math.Max(0, math.Min(1, float64(a.stamina)/float64(a.card.Stamina)))
	}
	combo := float64(e.sides[a.side].combo)
	switch eff.operation {
	case "score_get", "score_get_by_score_ratio":
	case "score_get_by_status_effect_type_grade":
		coefficient, supported := conditionalStatusCoefficient(eff.secondary)
		if !supported {
			e.err = unsupported(eff.id, "conditional status power coefficient is unknown")
			return 0
		}
		factor += coefficient * float64(e.grade(a, eff.secondary))
	case "score_get_by_strength_effect_count":
		seen := map[penum.StatusEffectType]bool{}
		for _, v := range a.effects {
			if e.isStrength(v.spec.status) && e.grade(a, v.spec.status) > 0 {
				seen[v.spec.status] = true
			}
		}
		limit := 9 + int(e.effectValue(a, penum.StatusEffectType_StatusEffectType_ScoreGetByStrengthEffectCountLimitUp))
		factor += .14 * float64(min(len(seen), limit))
	case "score_get_by_more_stamina":
		factor += .8 * ratio * ratio
	case "score_get_by_less_stamina":
		factor += .8 * (1 - ratio) * (1 - ratio)
	case "score_get_by_more_combo_count":
		factor += combo / 110
	case "score_get_by_less_combo_count":
		v := max(150-combo, 0) / 150
		factor += 2 * v * v
	case "score_get_by_more_fan_engage":
		v := e.coreFanRatio(a)
		factor += 1.8 * v * v
	case "score_get_by_less_fan_amount":
		// Name-based private-server rule: maximum +80% at zero total fans,
		// smoothly falling to zero extra power at twice the venue capacity.
		// Use character fan buckets, never attention-adjusted/allocated seats.
		if a.data.LightFans < 0 || a.data.MiddleFans < 0 || a.data.HeavyFans < 0 {
			e.err = unsupported(eff.id, "negative character fan count")
			return 0
		}
		total := float64(a.data.LightFans) + float64(a.data.MiddleFans) + float64(a.data.HeavyFans)
		fraction := min(1, max(0, total/(lowFanTargetCapacityMultiplier*float64(e.input.Quest.MaxCapacity))))
		factor += lowFanPowerMaximumBonus * (1 - fraction) * (1 - fraction)
	case "score_get_by_skill_activation_count":
		factor += .097 * float64(a.activations)
	case "score_get_by_more_stamina_use":
		factor += .00011 * float64(a.consumed)
	case "score_get_and_stamina_consumption_by_more_stamina_use":
		// Score workbook E117: +log10(current skill stamina / 900),
		// clamped from +0% at 900 to +100% at 9000. Unlike cumulative
		// stamina scaling, only this successful activation's debit counts.
		if s.currentStaminaCost > 900 {
			factor += min(1, math.Log10(float64(s.currentStaminaCost)/900))
		}
	case "score_get_by_skill_success_rate_up":
		// Workbook row121 uses success-rate units of 0.1%, independently
		// of the subsequent 0.1-percentage-point skill-power truncation.
		rate := math.Floor(min(max(e.successRate(a), 0), 2)*1000) / 1000
		factor += .48 * rate
	case "score_get_by_character_count":
		// Authored interpolation: four matching members gives the workbook's +100%;
		// earlier member counts scale by +25%, an explicitly unvalidated choice.
		count := 0
		for _, other := range e.actors {
			if !other.synthetic && other.side == a.side && other.data.GroupID == eff.text {
				count++
			}
		}
		factor += .25 * float64(min(count, 4))
	case "score_get_by_trigger":
		if matched, _ := e.matches(eff.conditional, a); matched {
			return float64(eff.conditionalPower) / 1000
		}
		return float64(eff.value) / 1000
	default:
		e.err = unsupported(eff.id, "conditional score formula is not supported")
		return 0
	}
	// Values are permil, so flooring here truncates power to 0.1 percentage point.
	return float64(e.scoreValue(float64(eff.value)*factor)) / 1000
}

func (e *simulation) successRate(a *actor) float64 {
	if a.synthetic {
		return 1
	}
	// Intentionally retain excess mental before tension and the caller's clamp.
	// The 5% success-up and 1.5% tension penalty are declared workbook inferences;
	// the audience/mental/pressure base was supplied by the user.
	base := 1000 * float64(a.card.Mental) * e.occupancy / float64(e.input.Quest.MentalThreshold) / float64(e.input.Pressure.WeightPermil)
	return max(0, base+float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_SkillSuccessRateUp))/1000-float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_TensionUp))*.3/1000)
}

func (e *simulation) criticalRate(a *actor) float64 {
	if a.synthetic {
		return 0
	}
	// Provisional player conjecture accepted by the user; do not invent a floor
	// or carry the older workbook's unproven 50% base cap into this formula.
	base := float64(a.card.Technique) / 1000 / float64(e.input.Quest.DifficultyLevel)
	if e.input.DisableBaseCriticalRate {
		base = 0
	}
	return min(1, max(0, base+float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_CriticalRateUp))/1000))
}

func (e *simulation) criticalFactor(a *actor, critical bool) float64 {
	if !critical {
		return 1
	}
	return 1.5 + float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_CriticalBonusPermilUp)+a.data.Bonuses[penum.PhotoAbilityType_PhotoAbilityType_CriticalScoreMultiply])/1000
}
func (e *simulation) jitter() float64 { return float64(950+e.rng.IntN(101)) / 1000 }

func (e *simulation) comboFactor(a *actor) float64 {
	combo := e.sides[a.side].combo
	bonus := int32(0)
	for _, row := range e.comboRows {
		if row.ComboCount > combo {
			break
		}
		bonus = max(0, row.AdvantagePermil-1000)
	}
	return 1 + float64(bonus)/1000*(1+float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_ComboScoreUp))/1000)
}

func (e *simulation) fanFactor(a *actor) float64 {
	// Attention/stealth redistribute a side's existing audience proportionally;
	// they neither create spectators nor change shared venue occupancy.
	weight := func(x *actor) float64 {
		return max(0, 1+float64(e.effectValue(x, penum.StatusEffectType_StatusEffectType_AudienceAmountIncrease)-e.effectValue(x, penum.StatusEffectType_StatusEffectType_AudienceAmountReduction))/1000)
	}
	total, weighted := 0.0, 0.0
	for _, other := range e.actors {
		if !other.synthetic && other.side == a.side {
			total += float64(other.card.AudienceAmount)
			weighted += float64(other.card.AudienceAmount) * weight(other)
		}
	}
	attendance := float64(a.card.AudienceAmount)
	if weighted > 0 {
		attendance = attendance * weight(a) * total / weighted
	}
	// Conservative lower-row lookup, clamped above the last master breakpoint.
	advantage := int32(1000)
	index := sort.Search(len(e.audienceRows), func(i int) bool { return float64(e.audienceRows[i].AudienceAmount) > attendance })
	if index > 0 {
		advantage = e.audienceRows[index-1].AdvantagePermil
	}
	return float64(advantage) / 1000
}

func (e *simulation) staminaCost(a *actor, s *skillState) int64 {
	base := float64(s.level.GetStamina()) + float64(a.card.Stamina)*float64(s.level.GetStaminaPermil())/1000
	// Boost's added cost is independent of its attribute effect and cannot be
	// removed as a weakness. Ordinary cost buffs/debuffs compose additively.
	boost := float64(e.grade(a, penum.StatusEffectType_StatusEffectType_VocalBoost)+e.grade(a, penum.StatusEffectType_StatusEffectType_DanceBoost)+e.grade(a, penum.StatusEffectType_StatusEffectType_VisualBoost)) * .01
	factor := max(0, 1+float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_StaminaConsumptionIncrease)-e.effectValue(a, penum.StatusEffectType_StatusEffectType_StaminaConsumptionReduction))/1000+boost)
	return e.scoreValue(base * float64(e.input.Quest.GetSkillStaminaWeightPermil()) / 1000 * factor)
}

func (e *simulation) authority(a *actor, attribute penum.AttributeType) float64 {
	if a.synthetic || a.card.Stamina <= 0 {
		return 0
	}
	staminaFactor := 1000.0
	if a.stamina <= 0 {
		staminaFactor = 250
	} else {
		for _, row := range e.staminaRows {
			if a.stamina*100 <= a.card.Stamina*int64(row.RemainStaminaRatioPercent) {
				staminaFactor = float64(row.SkillTargetParameterWeightPermil)
				break
			}
		}
	}
	// Workbook: floor(3.75% per success-up stage to a 0.1% boundary). Its
	// coefficient differs from actual success probability; tension adds nothing.
	successAdjustment := math.Floor(float64(e.effectValue(a, penum.StatusEffectType_StatusEffectType_SkillSuccessRateUp)) * .75)
	return float64(e.scoreValue(float64(e.attribute(a, attribute)) * (staminaFactor + successAdjustment) / 1000))
}
