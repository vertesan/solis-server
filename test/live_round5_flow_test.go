package test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"google.golang.org/protobuf/proto"
)

// These complete simulation scenarios use the integration harness but do not
// create or alter users: all prepared decks and skill state are in memory.
func TestRound5LiveFlow(t *testing.T) {
	t.Run("two qualifying recipients", func(t *testing.T) {
		in := liveEngineFixture(1)
		for _, card := range in.Users[0].UserDeck.Cards {
			card.Skills = nil
		}
		in.Targets["target-self"] = &pmaster.SkillTarget{Id: "target-self", Type: penum.SkillTargetType_SkillTargetType_Self}
		in.Targets["target-trigger-2"] = &pmaster.SkillTarget{Id: "target-trigger-2", Type: penum.SkillTargetType_SkillTargetType_Trigger}
		in.Triggers["tg-live_start"] = &pmaster.SkillTrigger{Id: "tg-live_start", Type: penum.SkillTriggerType_SkillTriggerType_LiveStart}
		in.Triggers["tg-someone_status_group-weekness"] = &pmaster.SkillTrigger{Id: "tg-someone_status_group-weekness", Type: penum.SkillTriggerType_SkillTriggerType_SomeoneStatusGroup}
		in.Setting.StatusEffectTypeWeaknessDownList = []penum.StatusEffectType{penum.StatusEffectType_StatusEffectType_VocalDown}
		weakness := liveStatusDefinition(in, "vocal_down", penum.SkillEfficacyType_SkillEfficacyType_VocalDown, 1, 20, 0, 10, "target-self")
		liveStatusSkill(in, "round5-start-weakness", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, weakness)
		for i := 0; i < 3; i++ {
			in.Users[0].UserDeck.Cards[i].Skills = []*papi.LiveDeckCardSkill{{Index: 3, SkillId: "round5-start-weakness", SkillLevel: 1, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card}}
		}
		const cleanse = "ef-weakness_effect_recovery-target-trigger-2"
		in.Efficacies[cleanse] = &pmaster.SkillEfficacy{Id: cleanse, Type: penum.SkillEfficacyType_SkillEfficacyType_WeaknessEffectRecovery, SkillTargetId: "target-trigger-2"}
		liveStatusSkill(in, "round5-cleanse", penum.SkillCategoryType_SkillCategoryType_Active, "", 0, cleanse)
		in.Skills["round5-cleanse"].Levels[0].SkillDetails[0].TriggerId = "tg-someone_status_group-weekness"
		in.Users[0].UserDeck.Cards[4].Skills = []*papi.LiveDeckCardSkill{{Index: 1, SkillId: "round5-cleanse", SkillLevel: 1, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card}}
		in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 5}}
		result := round5Simulate(t, in)
		active := result.GetCharts()[0].GetActivatedSkill()
		if !active.GetActivated() || len(active.GetDetails()) != 1 {
			t.Fatalf("conditional cleanse did not activate: %v", active)
		}
		targets := active.GetDetails()[0].GetTargetCardIndexes()
		if len(targets) != 2 || targets[0] == targets[1] {
			t.Fatalf("target-trigger-2 must select two distinct qualifying idols: %v", targets)
		}
		selected := map[int32]bool{}
		for _, index := range targets {
			if index < 1 || index > 3 {
				t.Fatalf("cleanse targeted healthy idol %d", index)
			}
			selected[index] = true
		}
		for index := int32(1); index <= 5; index++ {
			_, before := liveStatusAt(t, result.GetPreChart().GetCardStatuses(), index, penum.StatusEffectType_StatusEffectType_VocalDown)
			_, after := liveStatusAt(t, active.GetCardStatuses(), index, penum.StatusEffectType_StatusEffectType_VocalDown)
			if (before != nil) != (index <= 3) || (after != nil) != (index <= 3 && !selected[index]) {
				t.Fatalf("card %d incorrect conditional cleanse: before=%v after=%v targets=%v", index, before, after, targets)
			}
		}
	})

	for _, tc := range []struct {
		name  string
		chart penum.MusicChartType
		id    string
		slot  int32
	}{
		{"A probability failure", penum.MusicChartType_MusicChartType_ActiveSkill, "fixture-skill-2", 1},
		{"SP probability failure", penum.MusicChartType_MusicChartType_SpecialSkill, "fixture-skill-1", 2},
		{"all detail conditions fail", penum.MusicChartType_MusicChartType_ActiveSkill, "fixture-skill-2", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := liveEngineFixture(1)
			in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: tc.chart, Position: 1}, {Number: 2, Type: tc.chart, Position: 1}}
			level := in.Skills[tc.id].Levels[0]
			level.LimitCount, level.CoolTime = 1, 5
			failureType := penum.SkillFailureType_SkillFailureType_Miss
			if tc.name == "all detail conditions fail" {
				const trigger = "tg-combo_count_higher-999999"
				in.Triggers[trigger] = &pmaster.SkillTrigger{Id: trigger, Type: penum.SkillTriggerType_SkillTriggerType_Combo}
				level.SkillDetails[0].TriggerId = trigger
				failureType = penum.SkillFailureType_SkillFailureType_AllSkillEfficacyTriggerFail
			} else {
				in.Users[0].UserDeck.Cards[0].Mental = 0
			}
			result := round5Simulate(t, in)
			for _, chart := range result.GetCharts() {
				active := chart.GetActivatedSkill()
				if active.GetActivated() || active.GetStamina() != 0 || active.GetScore() != 0 || !round5HasFailure(active, 1, failureType) {
					t.Fatalf("failed attempt %d charged or activated: %v", chart.GetNumber(), active)
				}
				state, skill := round5SkillState(t, chart.GetCardStatuses(), 1, tc.slot)
				if state.GetStamina() != 500 || skill.GetCoolTime() != 0 || skill.GetRemainCount() != 1 {
					t.Fatalf("failed attempt %d mutated stamina/CT/use count: %v", chart.GetNumber(), state)
				}
				round5Descriptions(t, active)
			}
		})
	}

	// Round 5 GvG traces contain no probability Miss. These regressions encode
	// the user's explicit correction, not an inference from those captures.
	for _, bothFail := range []bool{false, true} {
		t.Run(fmt.Sprintf("battle fallback both_fail_%t", bothFail), func(t *testing.T) {
			in := liveEngineFixture(2)
			in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}}
			in.Skills["fixture-skill-2"].Levels[0].LimitCount = 1
			in.Skills["fixture-skill-2"].Levels[0].CoolTime = 5
			in.Users[0].UserDeck.Cards[0].Mental = 0
			if bothFail {
				in.Users[1].UserDeck.Cards[0].Mental = 0
			}
			result := round5Simulate(t, in)
			chart := result.GetCharts()[0]
			active := chart.GetActivatedSkill()
			if !round5HasFailure(active, 1, penum.SkillFailureType_SkillFailureType_Miss) {
				t.Fatalf("higher-attribute first side did not record its miss: %v", active)
			}
			failed, failedSkill := round5SkillState(t, chart.GetCardStatuses(), 1, 1)
			if failed.GetStamina() != 500 || failedSkill.GetCoolTime() != 0 || failedSkill.GetRemainCount() != 1 {
				t.Fatalf("first side paid for its failed authority attempt: %v", failed)
			}
			other, otherSkill := round5SkillState(t, chart.GetCardStatuses(), 6, 1)
			if bothFail {
				if active.GetActivated() || active.GetStamina() != 0 || active.GetScore() != 0 || !round5HasFailure(active, 6, penum.SkillFailureType_SkillFailureType_Miss) || other.GetStamina() != 500 || otherSkill.GetCoolTime() != 0 || otherSkill.GetRemainCount() != 1 {
					t.Fatalf("both-miss battle consumed resources or produced success: event=%v state=%v", active, other)
				}
			} else {
				if !active.GetActivated() || active.GetCardIndex() != 6 || active.GetStamina() != 10 || active.GetScore() <= 0 || other.GetStamina() != 490 || otherSkill.GetCoolTime() != 4 || otherSkill.GetRemainCount() != 0 {
					t.Fatalf("opponent did not receive/pay for fallback exactly once: event=%v state=%v", active, other)
				}
			}
			round5Descriptions(t, active)
		})
	}

	t.Run("photo CT follows native skill index", func(t *testing.T) {
		for _, tc := range []struct{ slot, cooldown int32 }{{1, 8}, {2, 6}, {3, 0}, {4, 10}} {
			t.Run(fmt.Sprintf("slot_%d", tc.slot), func(t *testing.T) {
				in := liveEngineFixture(1)
				in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}}
				in.Skills["fixture-skill-2"].Levels[0].CoolTime = 10
				in.Users[0].UserDeck.Cards[0].Skills = []*papi.LiveDeckCardSkill{{Index: tc.slot, SkillId: "fixture-skill-2", SkillLevel: 1, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card}}
				data := in.Cards[1]
				data.Bonuses = map[penum.PhotoAbilityType]int64{
					penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill1: 2,
					penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill2: 4,
					penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill3: 20,
				}
				in.Cards[1] = data
				result := round5Simulate(t, in)
				active := result.GetCharts()[0].GetActivatedSkill()
				_, skill := round5SkillState(t, active.GetCardStatuses(), 1, tc.slot)
				if !active.GetActivated() || skill.GetCoolTime() != tc.cooldown {
					t.Fatalf("slot %d cooldown=%d, want %d before end-of-beat tick", tc.slot, skill.GetCoolTime(), tc.cooldown)
				}
				round5Descriptions(t, active)
			})
		}
	})

	t.Run("successful self cooldown effect sees activation cooldown", func(t *testing.T) {
		in := liveEngineFixture(1)
		in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}}
		in.Targets["target-self"] = &pmaster.SkillTarget{Id: "target-self", Type: penum.SkillTargetType_SkillTargetType_Self}
		const effect = "ef-cool_time_reduction-3-target-self"
		in.Efficacies[effect] = &pmaster.SkillEfficacy{Id: effect, Type: penum.SkillEfficacyType_SkillEfficacyType_CoolTimeReduction, SkillTargetId: "target-self"}
		level := in.Skills["fixture-skill-2"].Levels[0]
		level.CoolTime = 10
		level.SkillDetails = append(level.SkillDetails, &pmaster.SkillDetail{EfficacyId: effect})
		data := in.Cards[1]
		data.Bonuses = map[penum.PhotoAbilityType]int64{penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill1: 2}
		in.Cards[1] = data
		result := round5Simulate(t, in)
		active := result.GetCharts()[0].GetActivatedSkill()
		_, skill := round5SkillState(t, active.GetCardStatuses(), 1, 1)
		if !active.GetActivated() || skill.GetCoolTime() != 5 {
			t.Fatalf("expected activation CT 10 - photo 2 - own effect 3 = 5, got %v", skill)
		}
	})

	t.Run("rejected reassignment candidate keeps migrated buffs suspended", func(t *testing.T) {
		in := round5EffectsFixture()
		const assignment = "ef-active_skill_chance_assignment-target-self-30"
		in.Efficacies[assignment] = &pmaster.SkillEfficacy{Id: assignment, Type: penum.SkillEfficacyType_SkillEfficacyType_ActiveSkillChanceAssignment, SkillTargetId: "target-self"}
		liveStatusSkill(in, "round5-start-reassignment", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, assignment)
		round5AddSkill(in, 3, 1, "fixture-skill-2")
		round5AddSkill(in, 3, 3, "round5-start-reassignment")
		round5AddSkill(in, 2, 1, "fixture-skill-2")

		buff := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 7, 20, 30, 20, "target-self")
		liveStatusSkill(in, "round5-center-start", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, buff)
		liveStatusSkill(in, "round5-center-active", penum.SkillCategoryType_SkillCategoryType_Active, "", 100, "ef-score_get-1000-chart_dependence")
		round5AddSkill(in, 1, 1, "round5-center-active")
		round5AddSkill(in, 1, 3, "round5-center-start")

		const migration = "ef-strength_effect_migration_before_active_skill-target-center"
		const beforeActive = "tg-before_active_skill_by_someone"
		in.Triggers[beforeActive] = &pmaster.SkillTrigger{Id: beforeActive, Type: penum.SkillTriggerType_SkillTriggerType_BeforeActiveSkillBySomeone}
		in.Efficacies[migration] = &pmaster.SkillEfficacy{Id: migration, Type: penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectMigrationBeforeActiveSkill, SkillTargetId: "target-center"}
		liveStatusSkill(in, "round5-migrate-once", penum.SkillCategoryType_SkillCategoryType_Passive, beforeActive, 0, migration)
		in.Skills["round5-migrate-once"].Levels[0].LimitCount = 1
		round5AddSkill(in, 4, 3, "round5-migrate-once")
		in.Chart = []*pmaster.MusicChartPattern{
			{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1},
			{Number: 2, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 3},
		}
		result := round5Simulate(t, in)
		_, initial := liveStatusAt(t, result.GetPreChart().GetCardStatuses(), 1, penum.StatusEffectType_StatusEffectType_VocalUp)
		if initial.GetGrade() != 7 {
			t.Fatal("center did not begin with the buff to suspend")
		}
		for i, wantActor := range []int32{1, 2} {
			chart := result.GetCharts()[i]
			active := chart.GetActivatedSkill()
			if !active.GetActivated() || active.GetCardIndex() != wantActor {
				t.Fatalf("chart %d expected actor %d, got %v", i+1, wantActor, active)
			}
			state, suspended := liveStatusAt(t, chart.GetCardStatuses(), 1, penum.StatusEffectType_StatusEffectType_VocalUp)
			if suspended != nil {
				t.Fatalf("chart %d restored a buff without a new accepted center opportunity: %v", i+1, suspended)
			}
			_, skill := round5SkillState(t, chart.GetCardStatuses(), 1, 1)
			if skill.GetCoolTime() != int32(99-i) {
				t.Fatalf("center was not rejected for the intended cooldown: %v", state)
			}
		}
	})
}

func round5Simulate(t *testing.T, in *live.Input) *papi.LiveResult {
	t.Helper()
	result, err := live.Simulate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := live.Simulate(context.Background(), in)
	if err != nil || !proto.Equal(result, again) {
		t.Fatalf("same-seed simulation/logging changed result: %v", err)
	}
	if err := checkLiveScoreConservation(result); err != nil {
		t.Fatal(err)
	}
	return result
}

func round5SkillState(t *testing.T, states []*papi.LiveCardStatus, card, slot int32) (*papi.LiveCardStatus, *papi.LiveSkillStatus) {
	t.Helper()
	for _, state := range states {
		if state.GetCardIndex() != card {
			continue
		}
		for _, skill := range state.GetSkillStatuses() {
			if skill.GetSkillIndex() == slot {
				return state, skill
			}
		}
	}
	t.Fatalf("missing card %d skill %d state", card, slot)
	return nil, nil
}

func round5HasFailure(skill *papi.LiveSkill, card int32, typ penum.SkillFailureType) bool {
	for _, failure := range skill.GetFailures() {
		if failure.GetCardIndex() == card && failure.GetType() == typ {
			return true
		}
	}
	return false
}

func round5Descriptions(t *testing.T, skill *papi.LiveSkill) {
	t.Helper()
	if len(skill.GetDescriptions()) == 0 {
		t.Fatalf("skill result lacks analysis descriptions: activated=%v failures=%v", skill.GetActivated(), skill.GetFailures())
	}
	for _, description := range skill.GetDescriptions() {
		if strings.TrimSpace(description.GetMessage()) == "" {
			t.Fatal("analysis description is blank")
		}
	}
}
