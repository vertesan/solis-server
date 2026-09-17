package test

import (
	"context"
	"math"
	"testing"

	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

// Check the formula against every permitted jitter value without depending on
// how many RNG draws prior effects or eligibility checks happen to consume.
func assertLiveScoreFormula(t *testing.T, score int64, base, fixed float64) {
	t.Helper()
	for jitter := 950; jitter <= 1050; jitter++ {
		want := int64(math.Floor(base*float64(jitter)/1000 + fixed))
		if score >= want-1 && score <= want+1 {
			return
		}
	}
	t.Fatalf("score %d does not match floor(%g * jitter + %g) for jitter 0.950..1.050", score, base, fixed)
}

func TestLiveConfirmedScoreFormulaScenario(t *testing.T) {
	setupTestUser(t)
	for _, kind := range []string{"active", "special", "passive", "beat"} {
		t.Run(kind, func(t *testing.T) {
			in := round5EffectsFixture()
			in.Quest.ActiveSkillWeightPermil = 1700
			in.Quest.SpecialSkillWeightPermil = 2300
			in.Quest.BeatVocalWeightPermil = 600
			in.Quest.BeatDanceWeightPermil = 200
			in.Quest.BeatVisualWeightPermil = 200
			in.AudienceAdvantages = []*pmaster.QuestAudienceAdvantage{{AudienceAmount: 0, AdvantagePermil: 1200}}
			in.ComboAdvantages = []*pmaster.ComboAdvantage{{ComboCount: 0, AdvantagePermil: 1000}, {ComboCount: 80, AdvantagePermil: 1300}}
			card := in.Users[0].UserDeck.Cards[0]
			card.Vocal, card.Dance, card.Visual = 100000, 140000, 120000
			card.Technique = 1000000 // Force criticals; probability is tested separately.
			data := in.Cards[1]
			data.CharacterAdvantagePermil = 1200
			data.Bonuses = map[penum.PhotoAbilityType]int64{
				penum.PhotoAbilityType_PhotoAbilityType_BeatScoreMultiply:         750,
				penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreMultiply:  750,
				penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreMultiply: 750,
				penum.PhotoAbilityType_PhotoAbilityType_CriticalScoreMultiply:     950,
				penum.PhotoAbilityType_PhotoAbilityType_BeatScoreAdd:              3456789,
				penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreAdd:       1234567,
				penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreAdd:      2345678,
			}
			in.Cards[1] = data
			var buffs []string
			for _, buff := range []struct {
				name  string
				typ   penum.SkillEfficacyType
				grade int32
			}{
				{"vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 2},
				{"score_up", penum.SkillEfficacyType_SkillEfficacyType_ScoreUp, 6},
				{"beat_score_up", penum.SkillEfficacyType_SkillEfficacyType_BeatScoreUp, 3},
				{"beat_boost", penum.SkillEfficacyType_SkillEfficacyType_BeatBoost, 4},
				{"active_skill_score_up", penum.SkillEfficacyType_SkillEfficacyType_ActiveSkillScoreUp, 8},
				{"special_skill_score_up", penum.SkillEfficacyType_SkillEfficacyType_SpecialSkillScoreUp, 7},
				{"passive_skill_score_up", penum.SkillEfficacyType_SkillEfficacyType_PassiveSkillScoreUp, 3},
				{"active_score_multiplier_add", penum.SkillEfficacyType_SkillEfficacyType_ActiveScoreMultiplierAdd, 4},
				{"special_score_multiplier_add", penum.SkillEfficacyType_SkillEfficacyType_SpecialScoreMultiplierAdd, 6},
				{"passive_score_multiplier_add", penum.SkillEfficacyType_SkillEfficacyType_PassiveScoreMultiplierAdd, 3},
				{"combo_score_up", penum.SkillEfficacyType_SkillEfficacyType_ComboScoreUp, 4},
				{"critical_bonus_permil_up", penum.SkillEfficacyType_SkillEfficacyType_CriticalBonusPermilUp, 12},
			} {
				buffs = append(buffs, liveStatusDefinition(in, buff.name, buff.typ, buff.grade, 30, 0, 200, "target-self"))
			}
			liveStatusSkill(in, "formula-initial-buffs", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, buffs...)
			round5AddSkill(in, 1, 3, "formula-initial-buffs")
			in.Chart = nil
			for number := int32(1); number <= 83; number++ {
				in.Chart = append(in.Chart, &pmaster.MusicChartPattern{Number: number, Type: penum.MusicChartType_MusicChartType_Beat})
			}
			category, chartType := penum.SkillCategoryType_SkillCategoryType_Active, penum.MusicChartType_MusicChartType_ActiveSkill
			power, weight, modifier, fixed := 10.5+.15*4, 1.7, 1+.75+.025*6+.05*8, float64(1234567)
			switch kind {
			case "special":
				category, chartType = penum.SkillCategoryType_SkillCategoryType_Special, penum.MusicChartType_MusicChartType_SpecialSkill
				power, weight, modifier, fixed = 10.5+.4*6, 2.3, 1+.75+.025*6+.03*7, 2345678
			case "passive":
				category, chartType = penum.SkillCategoryType_SkillCategoryType_Passive, penum.MusicChartType_MusicChartType_Beat
				power, weight, modifier, fixed = 10.5+.1*3, 1, 1+.025*6+.1*3, 0
			case "beat":
				chartType = penum.MusicChartType_MusicChartType_Beat
			}
			in.Chart = append(in.Chart, &pmaster.MusicChartPattern{Number: 84, Type: chartType, Position: 1})
			if kind != "beat" {
				const scoreEffect = "ef-score_get-10500-chart_dependence"
				in.Efficacies[scoreEffect] = &pmaster.SkillEfficacy{Id: scoreEffect, Type: penum.SkillEfficacyType_SkillEfficacyType_ScoreGet}
				liveStatusSkill(in, "formula-score", category, "", 0, scoreEffect)
				round5AddSkill(in, 1, 1, "formula-score")
			}
			result := round5Simulate(t, in)
			chart := result.Charts[len(result.Charts)-1]
			if result.Charts[len(result.Charts)-2].GetUserStatuses()[0].GetCurrentComboCount() != 83 {
				t.Fatal("formula fixture did not reach combo 83")
			}
			var score int64
			var critical bool
			base := 110000 * power * weight * modifier
			if kind == "beat" {
				score, critical = chart.Beats[0].GetScore(), chart.Beats[0].GetIsCritical()
				weighted := (float64(110000)*600 + 140000*200 + 120000*200) / 1000
				base = 8 * weighted / 84 * (1 + .75 + .025*6 + .1*3 + .05*4)
				fixed = 3456789
			} else {
				skill := chart.GetActivatedSkill()
				if kind == "passive" {
					for _, passive := range chart.GetActivatedPassiveSkills() {
						if passive.GetCardIndex() == 1 && passive.GetSkillIndex() == 1 {
							skill = passive
						}
					}
				}
				if !skill.GetActivated() {
					t.Fatal("formula skill did not activate")
				}
				score, critical = skill.GetScore(), skill.GetIsCritical()
			}
			if kind != "passive" && !critical {
				t.Fatal("formula fixture expected a critical A/SP/beat")
			}
			// Audience1.2 × combo(1+.3*(1+.1*4)) × critical(1.5+.95+.05*12)
			// × character1.2; fixed score is outside all of these multipliers.
			criticalFactor := 1.0
			if critical {
				criticalFactor = 1.5 + .95 + .05*12
			}
			base *= 1.2 * (1 + .3*(1+.1*4)) * criticalFactor * 1.2
			assertLiveScoreFormula(t, score, base, fixed)
			if result.GetUserInfos()[0].GetUserDeck().GetCards()[0].GetVocal() != 100000 || chart.GetCardStatuses()[0].GetVocal() != 110000 {
				t.Fatal("quest character advantage modified prepared or buffed attributes")
			}
		})
	}
}

func TestLiveBeatCountAndCharacterAdvantageScenario(t *testing.T) {
	setupTestUser(t)
	for _, sides := range []int{1, 2} {
		in := liveEngineFixture(sides)
		in.Chart = []*pmaster.MusicChartPattern{
			{Number: 1, Type: penum.MusicChartType_MusicChartType_Beat},
			{Number: 20, Type: penum.MusicChartType_MusicChartType_Unknown},
			{Number: 500, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1},
			{Number: 999, Type: penum.MusicChartType_MusicChartType_Beat},
		}
		data := in.Cards[1]
		data.Bonuses = map[penum.PhotoAbilityType]int64{penum.PhotoAbilityType_PhotoAbilityType_BeatScoreAdd: 10000}
		in.Cards[1] = data
		base := round5Simulate(t, in)
		data.CharacterAdvantagePermil = 2000
		in.Cards[1] = data
		boosted := round5Simulate(t, in)
		for _, pair := range [][2]*papi.LiveBeat{{base.Charts[0].Beats[0], boosted.Charts[0].Beats[0]}, {base.Charts[2].Beats[0], boosted.Charts[2].Beats[0]}} {
			// Exactly two beats per lane, independently of gaps, empty rows,
			// skill opportunities, or having five versus ten participating idols.
			assertLiveScoreFormula(t, pair[0].GetScore(), 8*1000.0/2, 10000)
			want := (pair[0].GetScore()-10000)*2 + 10000
			if pair[1].GetScore() < want || pair[1].GetScore() > want+1 {
				t.Fatalf("character bonus scaled fixed score: base=%d doubled=%d want=%d..%d", pair[0].GetScore(), pair[1].GetScore(), want, want+1)
			}
		}
		data.CharacterAdvantagePermil = 1000
		in.Cards[1] = data
		neutral := round5Simulate(t, in)
		if neutral.GetUserInfos()[0].GetUserResult().GetScore() != base.GetUserInfos()[0].GetUserResult().GetScore() {
			t.Fatal("omitted character advantage is not equivalent to neutral1000")
		}
		data.CharacterAdvantagePermil = -1
		in.Cards[1] = data
		if result, err := live.Simulate(context.Background(), in); result != nil || err == nil {
			t.Fatal("negative quest character advantage accepted")
		}
	}
}

func TestLiveReassignedScoreAttributeScenario(t *testing.T) {
	setupTestUser(t)
	in := round6AssignmentFixture(1)
	in.Quest.Position1AttributeType = penum.AttributeType_AttributeType_Vocal
	in.Quest.Position2AttributeType = penum.AttributeType_AttributeType_Dance
	recipient := in.Users[0].UserDeck.Cards[1]
	recipient.Vocal, recipient.Dance = 100000, 100
	data := in.Cards[2]
	data.CharacterAdvantagePermil = 1500
	data.Bonuses = map[penum.PhotoAbilityType]int64{penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreAdd: 12345}
	in.Cards[2] = data
	// An unconditional P on the recipient fires during the same Vocal A
	// opportunity, but must use that idol's own Dance lane instead.
	liveStatusSkill(in, "formula-reassigned-passive", penum.SkillCategoryType_SkillCategoryType_Passive, "", 0, "ef-score_get-1000-chart_dependence")
	round5AddSkill(in, 2, 3, "formula-reassigned-passive")
	result := round5Simulate(t, in)
	chart := result.GetCharts()[0]
	active := chart.GetActivatedSkill()
	if !active.GetActivated() || active.GetCardIndex() != 2 || active.GetIsCritical() {
		t.Fatalf("expected noncritical A reassigned from Vocal lane1 to Dance lane2: %v", active)
	}
	assertLiveScoreFormula(t, active.GetScore(), 100000*1.5, 12345)
	var passive *papi.LiveSkill
	for _, skill := range chart.GetActivatedPassiveSkills() {
		if skill.GetCardIndex() == 2 && skill.GetSkillIndex() == 3 {
			passive = skill
		}
	}
	if !passive.GetActivated() {
		t.Fatal("recipient's ordinary passive did not activate")
	}
	assertLiveScoreFormula(t, passive.GetScore(), 100*1.5, 0)

	t.Run("round6 response075 captured score factors", func(t *testing.T) {
		in := round6AssignmentFixture(1)
		const assignment = "ef-active_skill_chance_assignment-target-self-100"
		in.Efficacies[assignment] = &pmaster.SkillEfficacy{Id: assignment, Type: penum.SkillEfficacyType_SkillEfficacyType_ActiveSkillChanceAssignment, SkillTargetId: "target-self"}
		in.Skills["round6-assignment"].Levels[0].SkillDetails[0].EfficacyId = assignment
		in.Quest.Position1AttributeType = penum.AttributeType_AttributeType_Dance
		in.Quest.Position2AttributeType = penum.AttributeType_AttributeType_Visual
		in.Quest.MaxCapacity = 8409
		in.AudienceAdvantages = []*pmaster.QuestAudienceAdvantage{{AudienceAmount: 0, AdvantagePermil: 1000}, {AudienceAmount: 8009, AdvantagePermil: 1375}}
		in.ComboAdvantages = []*pmaster.ComboAdvantage{{ComboCount: 0, AdvantagePermil: 1000}, {ComboCount: 50, AdvantagePermil: 1250}}
		card := in.Users[0].UserDeck.Cards[1]
		card.Dance, card.Visual, card.Technique, card.AudienceAmount = 339718, 223383, 1000000, 8009
		data := in.Cards[2]
		data.Bonuses = map[penum.PhotoAbilityType]int64{
			penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreMultiply: 154 + 131 + 205,
			penum.PhotoAbilityType_PhotoAbilityType_CriticalScoreMultiply:    510 + 291,
		}
		in.Cards[2] = data
		buff := liveStatusDefinition(in, "score_up", penum.SkillEfficacyType_SkillEfficacyType_ScoreUp, 9, 20, 0, 100, "target-self")
		liveStatusSkill(in, "formula-captured-buff", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, buff)
		round5AddSkill(in, 2, 3, "formula-captured-buff")
		const scoreEffect = "ef-score_get-4000-chart_dependence"
		in.Efficacies[scoreEffect] = &pmaster.SkillEfficacy{Id: scoreEffect, Type: penum.SkillEfficacyType_SkillEfficacyType_ScoreGet}
		in.Skills["fixture-skill-2"].Levels[0].SkillDetails[0].EfficacyId = scoreEffect
		in.Chart = nil
		for number := int32(1); number <= 50; number++ {
			in.Chart = append(in.Chart, &pmaster.MusicChartPattern{Number: number, Type: penum.MusicChartType_MusicChartType_Beat})
		}
		in.Chart = append(in.Chart, &pmaster.MusicChartPattern{Number: 82, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1})
		// 075 chart82: Dance339718 rather than recipient lane2 Visual223383.
		// The captured alpha1.03 gives exactly9493125; our RNG is independent.
		base := float64(339718) * 4 * 1.715 * 1.375 * 1.25 * 2.301
		if captured := int64(math.Floor(base * 1.03)); captured != 9493125 {
			t.Fatalf("captured factor anchor drifted: %d", captured)
		}
		result := round5Simulate(t, in)
		active := result.GetCharts()[len(result.GetCharts())-1].GetActivatedSkill()
		if !active.GetActivated() || active.GetCardIndex() != 2 || !active.GetIsCritical() {
			t.Fatalf("capture fixture did not reassign its critical A to lane2: %v", active)
		}
		assertLiveScoreFormula(t, active.GetScore(), base, 0)
	})
}
