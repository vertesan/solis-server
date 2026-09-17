package test

import (
	"fmt"
	"testing"

	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

func round6AssignmentFixture(sides int) *live.Input {
	in := liveEngineFixture(sides)
	in.Targets["target-self"] = &pmaster.SkillTarget{Id: "target-self", Type: penum.SkillTargetType_SkillTargetType_Self}
	in.Triggers["tg-live_start"] = &pmaster.SkillTrigger{Id: "tg-live_start", Type: penum.SkillTriggerType_SkillTriggerType_LiveStart}
	const assignment = "ef-active_skill_chance_assignment-target-self-30"
	in.Efficacies[assignment] = &pmaster.SkillEfficacy{Id: assignment, Type: penum.SkillEfficacyType_SkillEfficacyType_ActiveSkillChanceAssignment, SkillTargetId: "target-self"}
	liveStatusSkill(in, "round6-assignment", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, assignment)
	round5AddSkill(in, 1, 3, "round6-assignment")
	in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}}
	return in
}

func TestLiveRound6ReassignmentScenario(t *testing.T) {
	for _, tc := range []struct {
		name string
		want int32
		edit func(*live.Input)
	}{
		{"highest chance attribute", 5, func(in *live.Input) {
			in.Users[0].UserDeck.Cards[4].Vocal = 2000
			in.Users[0].UserDeck.Cards[1].Dance = 9000
			in.Quest.Position2AttributeType = penum.AttributeType_AttributeType_Dance
		}},
		{"equal attributes use lower official lane", 2, func(in *live.Input) {}},
		{"skip highest without an A skill", 4, func(in *live.Input) {
			in.Users[0].UserDeck.Cards[4].Vocal = 3000
			in.Users[0].UserDeck.Cards[4].Skills = nil
			in.Users[0].UserDeck.Cards[3].Vocal = 2000
		}},
		{"skip highest without stamina", 3, func(in *live.Input) {
			in.Users[0].UserDeck.Cards[4].Vocal = 3000
			in.Users[0].UserDeck.Cards[4].Stamina = 1
			in.Users[0].UserDeck.Cards[2].Vocal = 2000
		}},
		{"skip highest in skill impossible status", 4, func(in *live.Input) {
			in.Users[0].UserDeck.Cards[4].Vocal = 3000
			in.Users[0].UserDeck.Cards[3].Vocal = 2000
			effect := liveStatusDefinition(in, "skill_impossible", penum.SkillEfficacyType_SkillEfficacyType_SkillImpossible, 1, 1, 0, 10, "target-self")
			liveStatusSkill(in, "round6-impossible", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, effect)
			round5AddSkill(in, 5, 3, "round6-impossible")
		}},
		{"recipient assignment does not recurse", 2, func(in *live.Input) {
			in.Users[0].UserDeck.Cards[1].Vocal = 2000
			round5AddSkill(in, 2, 3, "round6-assignment")
		}},
		{"fallback to original", 1, func(in *live.Input) {
			for _, card := range in.Users[0].UserDeck.Cards[1:] {
				card.Skills = nil
			}
		}},
		{"ready strongest wins despite reduced battle authority", 5, func(in *live.Input) {
			in.Users[0].UserDeck.Cards[4].Vocal = 2000
			const drain = "ef-stamina_consumption-400-target-self"
			in.Efficacies[drain] = &pmaster.SkillEfficacy{Id: drain, Type: penum.SkillEfficacyType_SkillEfficacyType_StaminaConsumption, SkillTargetId: "target-self"}
			liveStatusSkill(in, "round6-drain", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, drain)
			round5AddSkill(in, 5, 3, "round6-drain")
			in.StaminaConsumption = []*pmaster.QuestStaminaConsumption{{RemainStaminaRatioPercent: 25, SkillTargetParameterWeightPermil: 250}, {RemainStaminaRatioPercent: 100, SkillTargetParameterWeightPermil: 1000}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := round6AssignmentFixture(1)
			tc.edit(in)
			active := round5Simulate(t, in).GetCharts()[0].GetActivatedSkill()
			if !active.GetActivated() || active.GetCardIndex() != tc.want {
				t.Fatalf("expected reassigned actor %d: %v", tc.want, active)
			}
		})
	}
	t.Run("skip highest while its skill cools down", func(t *testing.T) {
		in := round6AssignmentFixture(1)
		in.Users[0].UserDeck.Cards[4].Vocal = 3000
		in.Users[0].UserDeck.Cards[3].Vocal = 2000
		in.Chart = append(in.Chart, &pmaster.MusicChartPattern{Number: 2, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1})
		result := round5Simulate(t, in)
		for event, want := range []int32{5, 4} {
			if active := result.GetCharts()[event].GetActivatedSkill(); !active.GetActivated() || active.GetCardIndex() != want {
				t.Fatalf("event %d expected actor %d: %v", event, want, active)
			}
		}
	})
	t.Run("battle authority and A score use original chance attribute", func(t *testing.T) {
		var scores []int64
		for _, laneAttribute := range []penum.AttributeType{penum.AttributeType_AttributeType_Dance, penum.AttributeType_AttributeType_Vocal} {
			in := round6AssignmentFixture(2)
			in.Quest.Position2AttributeType = laneAttribute
			in.Users[0].UserDeck.Cards[1].Vocal = 4000
			in.Users[0].UserDeck.Cards[1].Dance = 100
			in.Users[1].UserDeck.Cards[0].Vocal = 2000
			active := round5Simulate(t, in).GetCharts()[0].GetActivatedSkill()
			if !active.GetActivated() || active.GetCardIndex() != 2 || !round5HasFailure(active, 6, penum.SkillFailureType_SkillFailureType_OpponentActivation) {
				t.Fatalf("reassigned lane2's Vocal4000 must win lane1 Vocal chance over Vocal2000: %v", active)
			}
			scores = append(scores, active.GetScore())
		}
		if scores[0] != scores[1] {
			t.Fatalf("reassigned A score depended on recipient's own lane attribute: %v", scores)
		}
	})
}

func TestLiveRound6BeatBoostScenario(t *testing.T) {
	// Compare like-for-like complete runs: two ScoreUp stages (2.5% each)
	// must equal one temporary BeatBoost stage (5%), with identical RNG draws.
	for _, stages := range []int32{1, 5, 9} {
		t.Run(fmt.Sprintf("%d stages", stages), func(t *testing.T) {
			var results []*papi.LiveResult
			for _, boost := range []bool{false, true} {
				in := round5EffectsFixture()
				operation, typ, grade := "score_up", penum.SkillEfficacyType_SkillEfficacyType_ScoreUp, stages*2
				if boost {
					operation, typ, grade = "beat_boost", penum.SkillEfficacyType_SkillEfficacyType_BeatBoost, stages
				}
				effect := liveStatusDefinition(in, operation, typ, grade, 20, 0, 5, "target-self")
				liveStatusSkill(in, "round6-beat-bonus", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, effect)
				round5AddSkill(in, 1, 3, "round6-beat-bonus")
				in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_Beat}, {Number: 2, Type: penum.MusicChartType_MusicChartType_Beat}}
				results = append(results, round5Simulate(t, in))
			}
			for event, chart := range results[0].GetCharts() {
				for lane, beat := range chart.GetBeats() {
					got := results[1].GetCharts()[event].GetBeats()[lane]
					if got.GetScore() != beat.GetScore() || got.GetIsCritical() != beat.GetIsCritical() {
						t.Fatalf("5%% per BeatBoost stage mismatch: event=%d card=%d score-up=%v boost=%v", event, beat.GetCardIndex(), beat, got)
					}
				}
				for _, state := range results[1].GetCharts()[event].GetCardStatuses() {
					if state.GetStamina() != 500 {
						t.Fatalf("BeatBoost must not drain stamina: event=%d state=%v", event, state)
					}
				}
			}
		})
	}
}

func TestLiveRound6MigrationAuthorityScenario(t *testing.T) {
	for _, special := range []bool{false, true} {
		t.Run(fmt.Sprintf("special_%t", special), func(t *testing.T) {
			in := liveEngineFixture(2)
			for _, user := range in.Users {
				for _, card := range user.UserDeck.Cards {
					card.Skills = nil
				}
			}
			in.Targets["target-self"] = &pmaster.SkillTarget{Id: "target-self", Type: penum.SkillTargetType_SkillTargetType_Self}
			in.Targets["target-center"] = &pmaster.SkillTarget{Id: "target-center", Type: penum.SkillTargetType_SkillTargetType_Center}
			in.Triggers["tg-live_start"] = &pmaster.SkillTrigger{Id: "tg-live_start", Type: penum.SkillTriggerType_SkillTriggerType_LiveStart}
			in.Setting.StatusEffectTypeStrengthList = []penum.StatusEffectType{penum.StatusEffectType_StatusEffectType_VocalUp}
			buff := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 20, 20, 0, 10, "target-self")
			liveStatusSkill(in, "round6-authority-buff", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, buff)
			round5AddSkill(in, 1, 3, "round6-authority-buff")
			in.Users[0].UserDeck.Cards[0].Mental = 2000 // Apply buff before migration.
			name, typ, chartType, skillID, slot := "active", penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectMigrationBeforeActiveSkill, penum.MusicChartType_MusicChartType_ActiveSkill, "fixture-skill-2", int32(1)
			if special {
				name, typ, chartType, skillID, slot = "special", penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectMigrationBeforeSpecialSkill, penum.MusicChartType_MusicChartType_SpecialSkill, "fixture-skill-1", 2
			}
			migration := "ef-strength_effect_migration_before_" + name + "_skill-target-center"
			in.Efficacies[migration] = &pmaster.SkillEfficacy{Id: migration, Type: typ, SkillTargetId: "target-center"}
			liveStatusSkill(in, "round6-authority-migrate", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, migration)
			round5AddSkill(in, 2, 3, "round6-authority-migrate")
			round5AddSkill(in, 1, slot, skillID)
			in.Users[1].UserDeck.Cards[0].Skills = []*papi.LiveDeckCardSkill{{Index: slot, SkillId: skillID, SkillLevel: 1, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card}}
			in.Users[1].UserDeck.Cards[0].Vocal = 1500
			in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: chartType, Position: 1}}
			result := round5Simulate(t, in)
			_, suspended := liveStatusAt(t, result.GetPreChart().GetCardStatuses(), 1, penum.StatusEffectType_StatusEffectType_VocalUp)
			if suspended != nil {
				t.Fatal("authority fixture did not suspend the center's buff")
			}
			active := result.GetCharts()[0].GetActivatedSkill()
			if !active.GetActivated() || active.GetCardIndex() != 1 || !round5HasFailure(active, 6, penum.SkillFailureType_SkillFailureType_OpponentActivation) {
				t.Fatalf("restored Vocal 2000 must beat opposing Vocal 1500: %v", active)
			}
			_, restored := liveStatusAt(t, active.GetCardStatuses(), 1, penum.StatusEffectType_StatusEffectType_VocalUp)
			if restored.GetGrade() != 20 {
				t.Fatalf("migration was not restored before authority: %v", restored)
			}
		})
	}
	t.Run("transferred opportunity restores the recipient migration", func(t *testing.T) {
		in := round6AssignmentFixture(1)
		in.Targets["target-neighbor"] = &pmaster.SkillTarget{Id: "target-neighbor", Type: penum.SkillTargetType_SkillTargetType_Neighbor}
		in.Setting.StatusEffectTypeStrengthList = []penum.StatusEffectType{penum.StatusEffectType_StatusEffectType_VocalUp}
		in.Users[0].UserDeck.Cards[1].Vocal = 2000
		buff := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 20, 20, 0, 10, "target-self")
		liveStatusSkill(in, "round6-transfer-buff", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, buff)
		round5AddSkill(in, 2, 3, "round6-transfer-buff")
		const migration = "ef-strength_effect_migration_before_active_skill-target-neighbor"
		in.Efficacies[migration] = &pmaster.SkillEfficacy{Id: migration, Type: penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectMigrationBeforeActiveSkill, SkillTargetId: "target-neighbor"}
		liveStatusSkill(in, "round6-transfer-migrate", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, migration)
		round5AddSkill(in, 4, 3, "round6-transfer-migrate") // Left edge's only neighbor is lane2.
		in.Chart = append(in.Chart, &pmaster.MusicChartPattern{Number: 2, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 2})
		result := round5Simulate(t, in)
		first := result.GetCharts()[0].GetActivatedSkill()
		if !first.GetActivated() || first.GetCardIndex() != 2 {
			t.Fatalf("lane2 should receive lane1's transferred chance: %v", first)
		}
		_, early := liveStatusAt(t, first.GetCardStatuses(), 2, penum.StatusEffectType_StatusEffectType_VocalUp)
		if early.GetGrade() != 20 || early.GetRemainEffectedChartCount() != 10 {
			t.Fatalf("transferred chance must restore the recipient's migration: %v", early)
		}
		second := result.GetCharts()[1]
		_, restored := liveStatusAt(t, second.GetCardStatuses(), 2, penum.StatusEffectType_StatusEffectType_VocalUp)
		if restored.GetGrade() != 20 || restored.GetRemainEffectedChartCount() != 8 || !round5HasFailure(second.GetActivatedSkill(), 2, penum.SkillFailureType_SkillFailureType_InCoolTime) {
			t.Fatalf("restored effects must tick normally while the recipient's skill cools down: %v", second)
		}
	})
}
