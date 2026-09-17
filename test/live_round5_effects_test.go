package test

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

func round5EffectsFixture() *live.Input {
	in := liveEngineFixture(1)
	in.Targets["target-self"] = &pmaster.SkillTarget{Id: "target-self", Type: penum.SkillTargetType_SkillTargetType_Self}
	in.Targets["target-center"] = &pmaster.SkillTarget{Id: "target-center", Type: penum.SkillTargetType_SkillTargetType_Center}
	in.Triggers["tg-live_start"] = &pmaster.SkillTrigger{Id: "tg-live_start", Type: penum.SkillTriggerType_SkillTriggerType_LiveStart}
	in.Setting.StatusEffectTypeStrengthList = []penum.StatusEffectType{
		penum.StatusEffectType_StatusEffectType_VocalUp,
		penum.StatusEffectType_StatusEffectType_TensionUp,
		penum.StatusEffectType_StatusEffectType_SkillSuccessRateUp,
		penum.StatusEffectType_StatusEffectType_LimitBreakTensionUp,
	}
	for _, card := range in.Users[0].UserDeck.Cards {
		card.Skills = nil
	}
	return in
}

func round5AddSkill(in *live.Input, card int, index int32, id string) {
	in.Users[0].UserDeck.Cards[card-1].Skills = append(in.Users[0].UserDeck.Cards[card-1].Skills,
		&papi.LiveDeckCardSkill{Index: index, SkillId: id, SkillLevel: 1, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card})
}

func TestLiveRound5EffectScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	t.Run("migration pauses duration until matching recipient chance", func(t *testing.T) {
		for _, category := range []penum.SkillCategoryType{penum.SkillCategoryType_SkillCategoryType_Active, penum.SkillCategoryType_SkillCategoryType_Special} {
			in := round5EffectsFixture()
			buff := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 7, 20, 30, 10, "target-self")
			liveStatusSkill(in, "fixture-start", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, buff)
			round5AddSkill(in, 1, 3, "fixture-start")
			name, typ := "active", penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectMigrationBeforeActiveSkill
			chartType, otherType := penum.MusicChartType_MusicChartType_ActiveSkill, penum.MusicChartType_MusicChartType_SpecialSkill
			if category == penum.SkillCategoryType_SkillCategoryType_Special {
				name, typ = "special", penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectMigrationBeforeSpecialSkill
				chartType, otherType = otherType, chartType
			}
			migration := "ef-strength_effect_migration_before_" + name + "_skill-target-center"
			in.Efficacies[migration] = &pmaster.SkillEfficacy{Id: migration, Type: typ, SkillTargetId: "target-center"}
			liveStatusSkill(in, "fixture-migrate", penum.SkillCategoryType_SkillCategoryType_Active, "", 0, migration)
			round5AddSkill(in, 3, 1, "fixture-migrate")
			round5AddSkill(in, 1, 1, "fixture-skill-2")
			round5AddSkill(in, 1, 2, "fixture-skill-1")
			in.Chart = []*pmaster.MusicChartPattern{
				{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 3},
				{Number: 2, Type: penum.MusicChartType_MusicChartType_Beat},
				{Number: 3, Type: otherType, Position: 1},
				{Number: 4, Type: chartType, Position: 1},
			}
			result, err := live.Simulate(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				_, status := liveStatusAt(t, result.Charts[i].CardStatuses, 1, penum.StatusEffectType_StatusEffectType_VocalUp)
				if status != nil {
					t.Fatalf("%s migration restored at wrong opportunity %d: %v", name, i+1, status)
				}
			}
			_, restored := liveStatusAt(t, result.Charts[3].ActivatedSkill.CardStatuses, 1, penum.StatusEffectType_StatusEffectType_VocalUp)
			if restored.GetGrade() != 7 || restored.GetRemainEffectedChartCount() != 10 || len(restored.GetStatusReasons()) != 1 || restored.StatusReasons[0].CardIndex != 3 || restored.StatusReasons[0].SkillIndex != 1 {
				t.Fatalf("%s migration lost lifetime/stages/new provenance: %v", name, restored)
			}
		}
	})
	t.Run("conversion uses destination caps and extension touches every instance", func(t *testing.T) {
		in := round5EffectsFixture()
		short := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 8, 20, 30, 2, "target-self")
		long := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 8, 20, 30, 8, "target-self")
		liveStatusDefinition(in, "tension_up", penum.SkillEfficacyType_SkillEfficacyType_TensionUp, 1, 10, 20, 1, "target-self")
		extend := "ef-strength_effect_count_increase-4-target-self"
		convert := "ef-status_effect_change-target-self-vocal_up-tension_up"
		in.Efficacies[extend] = &pmaster.SkillEfficacy{Id: extend, Type: penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectCountIncrease, SkillTargetId: "target-self"}
		in.Efficacies[convert] = &pmaster.SkillEfficacy{Id: convert, Type: penum.SkillEfficacyType_SkillEfficacyType_StatusEffectChange, SkillTargetId: "target-self"}
		liveStatusSkill(in, "fixture-start", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, short, long)
		liveStatusSkill(in, "fixture-convert", penum.SkillCategoryType_SkillCategoryType_Active, "", 100, extend, convert)
		round5AddSkill(in, 1, 3, "fixture-start")
		round5AddSkill(in, 1, 1, "fixture-convert")
		in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}}
		for i := 2; i <= 7; i++ {
			in.Chart = append(in.Chart, &pmaster.MusicChartPattern{Number: int32(i), Type: penum.MusicChartType_MusicChartType_Beat})
		}
		result, err := live.Simulate(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		_, converted := liveStatusAt(t, result.Charts[0].ActivatedSkill.CardStatuses, 1, penum.StatusEffectType_StatusEffectType_TensionUp)
		if converted.GetGrade() != 10 || converted.GetMaxGrade() != 10 || converted.GetLimitBreakMaxGrade() != 20 {
			t.Fatalf("conversion did not use tension's master caps: %v", converted)
		}
		_, overlapping := liveStatusAt(t, result.Charts[3].CardStatuses, 1, penum.StatusEffectType_StatusEffectType_TensionUp)
		if overlapping.GetGrade() != 10 {
			t.Fatalf("short instance was not extended: %v", overlapping)
		}
		_, expired := liveStatusAt(t, result.Charts[6].CardStatuses, 1, penum.StatusEffectType_StatusEffectType_TensionUp)
		if expired.GetGrade() != 8 {
			t.Fatalf("short instance did not expire independently: %v", expired)
		}
	})
	t.Run("current skill stamina uses logarithmic score scaling", func(t *testing.T) {
		for _, cost := range []int64{0, 450, 900, 2000, 6497, 9000, 10000} {
			in := round5EffectsFixture()
			card := in.Users[0].UserDeck.Cards[0]
			card.Stamina = 30000
			effect := "ef-score_get_and_stamina_consumption_by_more_stamina_use-14000-chart_dependence"
			in.Efficacies[effect] = &pmaster.SkillEfficacy{Id: effect, Type: penum.SkillEfficacyType_SkillEfficacyType_ScoreGetAndStaminaConsumptionByMoreStaminaUse}
			liveStatusSkill(in, "fixture-cost", penum.SkillCategoryType_SkillCategoryType_Special, "", 0, effect)
			in.Skills["fixture-cost"].Levels[0].Stamina = int32(cost)
			round5AddSkill(in, 1, 1, "fixture-cost")
			in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_SpecialSkill, Position: 1}, {Number: 2, Type: penum.MusicChartType_MusicChartType_SpecialSkill, Position: 1}}
			actual, err := live.Simulate(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			factor := 1.0
			if cost > 900 {
				factor += min(1, math.Log10(float64(cost)/900))
			}
			power := int64(math.Floor(14000 * factor))
			baseline := fmt.Sprintf("ef-score_get-%d-chart_dependence", power)
			in.Efficacies[baseline] = &pmaster.SkillEfficacy{Id: baseline, Type: penum.SkillEfficacyType_SkillEfficacyType_ScoreGet}
			in.Skills["fixture-cost"].Levels[0].SkillDetails[0].EfficacyId = baseline
			want, err := live.Simulate(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			for i, chart := range actual.Charts {
				if chart.ActivatedSkill.Score != want.Charts[i].ActivatedSkill.Score || chart.ActivatedSkill.Stamina != cost {
					t.Fatalf("cost %d activation %d has incorrect power or debit: %v", cost, i, chart.ActivatedSkill)
				}
			}
		}
	})
	t.Run("photo modifiers preserve status filters and apply only once", func(t *testing.T) {
		in := round5EffectsFixture()
		vocal := penum.StatusEffectType_StatusEffectType_VocalUp
		dance := penum.StatusEffectType_StatusEffectType_DanceUp
		in.Setting.StatusEffectTypeStrengthList = append(in.Setting.StatusEffectTypeStrengthList, dance)
		modifier := func(typ penum.PhotoAbilityType, amount int32) live.StrengthModifier {
			return live.StrengthModifier{Type: typ, StatusTypes: []penum.StatusEffectType{vocal}, Amount: amount}
		}
		source, target := in.Cards[3], in.Cards[1]
		source.StrengthModifiers = []live.StrengthModifier{
			modifier(penum.PhotoAbilityType_PhotoAbilityType_AffectStrengthEffectValueIncrease, 2),
			modifier(penum.PhotoAbilityType_PhotoAbilityType_AffectStrengthEffectCountIncrease, 3),
			modifier(penum.PhotoAbilityType_PhotoAbilityType_ReceivedStrengthEffectValueIncrease, 20),
		}
		target.StrengthModifiers = []live.StrengthModifier{
			modifier(penum.PhotoAbilityType_PhotoAbilityType_ReceivedStrengthEffectValueIncrease, 3),
			modifier(penum.PhotoAbilityType_PhotoAbilityType_ReceivedStrengthEffectCountIncrease, 4),
			modifier(penum.PhotoAbilityType_PhotoAbilityType_AffectStrengthEffectValueIncrease, 20),
		}
		in.Cards[3], in.Cards[1] = source, target
		vocalEffect := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 5, 20, 30, 2, "target-center")
		danceEffect := liveStatusDefinition(in, "dance_up", penum.SkillEfficacyType_SkillEfficacyType_DanceUp, 5, 20, 30, 2, "target-center")
		liveStatusSkill(in, "fixture-start", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, vocalEffect, danceEffect)
		round5AddSkill(in, 3, 3, "fixture-start")
		migration := "ef-strength_effect_migration_before_active_skill-target-center"
		in.Efficacies[migration] = &pmaster.SkillEfficacy{Id: migration, Type: penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectMigrationBeforeActiveSkill, SkillTargetId: "target-center"}
		liveStatusSkill(in, "fixture-migrate", penum.SkillCategoryType_SkillCategoryType_Active, "", 0, migration)
		round5AddSkill(in, 3, 1, "fixture-migrate")
		round5AddSkill(in, 1, 1, "fixture-skill-2")
		in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 3}, {Number: 2, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}}
		result, err := live.Simulate(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		for _, states := range [][]*papi.LiveCardStatus{result.PreChart.CardStatuses, result.Charts[1].ActivatedSkill.CardStatuses} {
			_, vocalStatus := liveStatusAt(t, states, 1, vocal)
			_, danceStatus := liveStatusAt(t, states, 1, dance)
			if vocalStatus.GetGrade() != 10 || vocalStatus.GetRemainEffectedChartCount() != 9 || danceStatus.GetGrade() != 5 || danceStatus.GetRemainEffectedChartCount() != 2 {
				t.Fatalf("photo restriction, direction or repeated application incorrect: vocal=%v dance=%v", vocalStatus, danceStatus)
			}
		}
	})
	t.Run("transfer preserves existing provenance and rewrites moved provenance", func(t *testing.T) {
		in := round5EffectsFixture()
		in.Targets["target-all"] = &pmaster.SkillTarget{Id: "target-all", Type: penum.SkillTargetType_SkillTargetType_All}
		vocal := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 5, 20, 30, 20, "target-all")
		liveStatusSkill(in, "fixture-start", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, vocal)
		round5AddSkill(in, 5, 3, "fixture-start")
		assignment := "ef-strength_effect_assignment_all-target-center"
		in.Efficacies[assignment] = &pmaster.SkillEfficacy{Id: assignment, Type: penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectAssignmentAll, SkillTargetId: "target-center"}
		liveStatusSkill(in, "fixture-transfer", penum.SkillCategoryType_SkillCategoryType_Active, "", 0, assignment)
		round5AddSkill(in, 3, 1, "fixture-transfer")
		in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 3}}
		result, err := live.Simulate(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		_, old := liveStatusAt(t, result.Charts[0].ActivatedSkill.CardStatuses, 3, penum.StatusEffectType_StatusEffectType_VocalUp)
		_, moved := liveStatusAt(t, result.Charts[0].ActivatedSkill.CardStatuses, 1, penum.StatusEffectType_StatusEffectType_VocalUp)
		if old != nil || moved.GetGrade() != 10 || moved.GetRemainEffectedChartCount() != 20 || len(moved.GetStatusReasons()) != 2 || moved.StatusReasons[0].CardIndex != 5 || moved.StatusReasons[0].SkillIndex != 3 || moved.StatusReasons[1].CardIndex != 3 || moved.StatusReasons[1].SkillIndex != 1 {
			t.Fatalf("transfer lost lifetime, provenance, or source cleanup: old=%v moved=%v", old, moved)
		}
		if result.Charts[0].ActivatedSkill.Details[0].GetValue() != 0 {
			t.Fatal("transfer detail should not expose moved instance count")
		}
	})
}
