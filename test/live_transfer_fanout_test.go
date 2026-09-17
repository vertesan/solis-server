package test

import (
	"context"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"google.golang.org/protobuf/proto"
)

func liveTransferFixture(all bool) *live.Input {
	in := round5EffectsFixture()
	const target = "target-character_type-1-2"
	in.Targets[target] = &pmaster.SkillTarget{Id: target, Type: penum.SkillTargetType_SkillTargetType_CardType}
	for _, index := range []int32{1, 2} {
		data := in.Cards[index]
		data.Type = penum.CardType_CardType_Appeal
		in.Cards[index] = data
	}
	in.Setting.StatusEffectTypeStrengthList = append(in.Setting.StatusEffectTypeStrengthList, penum.StatusEffectType_StatusEffectType_DanceUp)
	vocal := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 3, 20, 30, 5, "target-self")
	dance := liveStatusDefinition(in, "dance_up", penum.SkillEfficacyType_SkillEfficacyType_DanceUp, 4, 20, 30, 8, "target-self")
	liveStatusSkill(in, "fixture-transfer-source", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, vocal, dance)
	round5AddSkill(in, 5, 3, "fixture-transfer-source")
	operation, typ := "strength_effect_assignment-vocal_up", penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectAssignment
	if all {
		operation, typ = "strength_effect_assignment_all", penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectAssignmentAll
	}
	assignment := "ef-" + operation + "-" + target
	in.Efficacies[assignment] = &pmaster.SkillEfficacy{Id: assignment, Type: typ, SkillTargetId: target}
	liveStatusSkill(in, "fixture-transfer", penum.SkillCategoryType_SkillCategoryType_Active, "", 0, assignment)
	round5AddSkill(in, 5, 1, "fixture-transfer")
	in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 5}}
	return in
}

func TestLiveTransferFanoutScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	vocal := penum.StatusEffectType_StatusEffectType_VocalUp
	dance := penum.StatusEffectType_StatusEffectType_DanceUp
	for _, all := range []bool{false, true} {
		name := "selected strength"
		if all {
			name = "all strengths"
		}
		t.Run(name, func(t *testing.T) {
			in := liveTransferFixture(all)
			for _, index := range []int32{1, 2, 5} {
				data := in.Cards[index]
				value, duration := int32(10), int32(10)
				if index == 5 {
					value, duration = 2, 3
				}
				data.StrengthModifiers = []live.StrengthModifier{
					{Type: penum.PhotoAbilityType_PhotoAbilityType_ReceivedStrengthEffectValueIncrease, StatusTypes: []penum.StatusEffectType{vocal}, Amount: value},
					{Type: penum.PhotoAbilityType_PhotoAbilityType_ReceivedStrengthEffectCountIncrease, StatusTypes: []penum.StatusEffectType{vocal}, Amount: duration},
				}
				in.Cards[index] = data
			}
			old := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 1, 20, 30, 20, "target-character_type-1-2")
			liveStatusSkill(in, "fixture-transfer-existing", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, old)
			round5AddSkill(in, 3, 3, "fixture-transfer-existing")
			extend := "ef-strength_effect_count_increase-4-target-self"
			in.Efficacies[extend] = &pmaster.SkillEfficacy{Id: extend, Type: penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectCountIncrease, SkillTargetId: "target-self"}
			liveStatusSkill(in, "fixture-transfer-extend", penum.SkillCategoryType_SkillCategoryType_Active, "", 0, extend)
			round5AddSkill(in, 1, 1, "fixture-transfer-extend")
			in.Chart = append(in.Chart, &pmaster.MusicChartPattern{Number: 2, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1})
			for number := int32(3); number <= 12; number++ {
				in.Chart = append(in.Chart, &pmaster.MusicChartPattern{Number: number, Type: penum.MusicChartType_MusicChartType_Beat})
			}
			result, err := live.Simulate(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			activation := result.Charts[0].ActivatedSkill
			if !activation.Activated || len(activation.Details) != 1 || len(activation.Details[0].TargetCardIndexes) != 2 || activation.Details[0].TargetCardIndexes[0] != 1 || activation.Details[0].TargetCardIndexes[1] != 2 || activation.Details[0].Value != 0 {
				t.Fatalf("transfer did not report both recipients in official lane order: %v", activation)
			}
			for _, index := range []int32{1, 2} {
				_, status := liveStatusAt(t, activation.CardStatuses, index, vocal)
				if status.GetGrade() != 16 || status.GetRemainEffectedChartCount() != 30 || len(status.GetStatusReasons()) != 2 || status.StatusReasons[0].CardIndex != 3 || status.StatusReasons[0].SkillIndex != 3 || status.StatusReasons[1].CardIndex != 5 || status.StatusReasons[1].SkillIndex != 1 {
					t.Fatalf("recipient %d lost stages, photos, lifetime or provenance: %v", index, status)
				}
				_, transferredDance := liveStatusAt(t, activation.CardStatuses, index, dance)
				if (transferredDance != nil) != all || all && transferredDance.GetGrade() != 4 {
					t.Fatalf("recipient %d selective/all transfer mismatch: %v", index, transferredDance)
				}
			}
			_, sourceVocal := liveStatusAt(t, activation.CardStatuses, 5, vocal)
			_, sourceDance := liveStatusAt(t, activation.CardStatuses, 5, dance)
			if sourceVocal != nil || (sourceDance == nil) != all {
				t.Fatalf("source cleanup did not respect selected strength types: vocal=%v dance=%v", sourceVocal, sourceDance)
			}
			// Only recipient 1 extended its cloned instances. Recipient 2's
			// transferred buff expires after eight charts, not after twelve.
			_, extended := liveStatusAt(t, result.Charts[7].CardStatuses, 1, vocal)
			_, expired := liveStatusAt(t, result.Charts[7].CardStatuses, 2, vocal)
			_, later := liveStatusAt(t, result.Charts[11].CardStatuses, 1, vocal)
			if extended.GetGrade() != 16 || expired.GetGrade() != 11 || later.GetGrade() != 11 {
				t.Fatalf("recipient instances share mutable lifetimes: extended=%v expired=%v later=%v", extended, expired, later)
			}
			_, original := liveStatusAt(t, result.PreChart.CardStatuses, 5, vocal)
			if original.GetGrade() != 5 || original.GetRemainEffectedChartCount() != 8 || original.StatusReasons[0].SkillIndex != 3 {
				t.Fatalf("transfer mutated a historical source snapshot: %v", original)
			}

			// Conversion must mutate only one recipient's copied effectSpec.
			convert := "ef-status_effect_change-target-self-vocal_up-tension_up"
			liveStatusDefinition(in, "tension_up", penum.SkillEfficacyType_SkillEfficacyType_TensionUp, 1, 10, 20, 1, "target-self")
			in.Efficacies[convert] = &pmaster.SkillEfficacy{Id: convert, Type: penum.SkillEfficacyType_SkillEfficacyType_StatusEffectChange, SkillTargetId: "target-self"}
			in.Skills["fixture-transfer-extend"].Levels[0].SkillDetails = []*pmaster.SkillDetail{{EfficacyId: convert}}
			converted, err := live.Simulate(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			_, untouched := liveStatusAt(t, converted.Charts[1].ActivatedSkill.CardStatuses, 2, vocal)
			_, convertedStatus := liveStatusAt(t, converted.Charts[1].ActivatedSkill.CardStatuses, 1, penum.StatusEffectType_StatusEffectType_TensionUp)
			if untouched.GetGrade() != 16 || convertedStatus.GetGrade() != 10 {
				t.Fatalf("conversion affected the other transfer recipient: untouched=%v converted=%v", untouched, convertedStatus)
			}
		})
	}
	t.Run("self and empty target sets preserve source once", func(t *testing.T) {
		for _, recipients := range [][]int32{{}, {5}, {1, 5}} {
			in := liveTransferFixture(true)
			for index, data := range in.Cards {
				data.Type = penum.CardType_CardType_Support
				for _, recipient := range recipients {
					if recipient == index {
						data.Type = penum.CardType_CardType_Appeal
					}
				}
				in.Cards[index] = data
			}
			result, err := live.Simulate(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			states := result.Charts[0].ActivatedSkill.CardStatuses
			_, retained := liveStatusAt(t, states, 5, vocal)
			if retained.GetGrade() != 3 || len(retained.GetStatusReasons()) != 1 || retained.StatusReasons[0].SkillIndex != 3 {
				t.Fatalf("source-inclusive/empty transfer changed source: recipients=%v status=%v", recipients, retained)
			}
			if len(recipients) == 2 {
				_, copied := liveStatusAt(t, states, 1, vocal)
				if copied.GetGrade() != 3 || copied.StatusReasons[0].SkillIndex != 1 {
					t.Fatalf("source-inclusive transfer lost other recipient: %v", copied)
				}
			}
		}
	})
	t.Run("recipient capacity failure returns no partial simulation", func(t *testing.T) {
		in := liveTransferFixture(true)
		buff := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 1, 20, 30, 20, "target-self")
		effects := make([]string, 1023)
		for i := range effects {
			effects[i] = buff
		}
		liveStatusSkill(in, "fixture-transfer-crowded", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, effects...)
		round5AddSkill(in, 2, 3, "fixture-transfer-crowded")
		beforeUser := proto.Clone(in.Users[0])
		beforeSkill := proto.Clone(in.Skills["fixture-transfer-crowded"])
		result, err := live.Simulate(ctx, in)
		if result != nil || err == nil || !strings.Contains(err.Error(), "too many transferred live statuses") {
			t.Fatalf("overflowing second recipient was not rejected atomically: result=%t err=%v", result != nil, err)
		}
		if !proto.Equal(beforeUser, in.Users[0]) || !proto.Equal(beforeSkill, in.Skills["fixture-transfer-crowded"]) {
			t.Fatal("failed transfer mutated immutable simulation inputs")
		}
		// Retrying a now-valid fixture starts cleanly and transfers to both.
		in.Skills["fixture-transfer-crowded"].Levels[0].SkillDetails = in.Skills["fixture-transfer-crowded"].Levels[0].SkillDetails[:1022]
		result, err = live.Simulate(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		for _, index := range []int32{1, 2} {
			_, status := liveStatusAt(t, result.Charts[0].ActivatedSkill.CardStatuses, index, dance)
			if status.GetGrade() != 4 {
				t.Fatalf("valid retry lost recipient %d: %v", index, status)
			}
		}
	})
}
