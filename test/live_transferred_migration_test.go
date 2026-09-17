package test

import (
	"fmt"
	"testing"

	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

func TestLiveTransferredMigrationScenario(t *testing.T) {
	for _, opponentVocal := range []int64{3000, 5000} {
		t.Run(fmt.Sprintf("opponent_vocal_%d", opponentVocal), func(t *testing.T) {
			in := round6AssignmentFixture(2)
			in.Targets["target-all"] = &pmaster.SkillTarget{Id: "target-all", Type: penum.SkillTargetType_SkillTargetType_All}
			in.Setting.StatusEffectTypeStrengthList = []penum.StatusEffectType{penum.StatusEffectType_StatusEffectType_VocalUp}
			const assignment = "ef-active_skill_chance_assignment-target-self-1"
			in.Efficacies[assignment] = &pmaster.SkillEfficacy{Id: assignment, Type: penum.SkillEfficacyType_SkillEfficacyType_ActiveSkillChanceAssignment, SkillTargetId: "target-self"}
			buff := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 20, 20, 0, 10, "target-self")
			in.Skills["round6-assignment"].Levels[0].SkillDetails = []*pmaster.SkillDetail{{EfficacyId: assignment}, {EfficacyId: buff}}
			liveStatusSkill(in, "recipient-start-buff", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, buff)
			round5AddSkill(in, 2, 3, "recipient-start-buff")
			in.Users[0].UserDeck.Cards[1].Vocal = 2000
			in.Users[1].UserDeck.Cards[0].Vocal = opponentVocal
			const migration = "ef-strength_effect_migration_before_active_skill-target-all"
			in.Efficacies[migration] = &pmaster.SkillEfficacy{Id: migration, Type: penum.SkillEfficacyType_SkillEfficacyType_StrengthEffectMigrationBeforeActiveSkill, SkillTargetId: "target-all"}
			liveStatusSkill(in, "suspend-start-buffs", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, migration)
			round5AddSkill(in, 4, 3, "suspend-start-buffs")
			in.Chart = append(in.Chart, &pmaster.MusicChartPattern{Number: 2, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1})
			result := round5Simulate(t, in)
			for _, index := range []int32{1, 2} {
				_, status := liveStatusAt(t, result.GetPreChart().GetCardStatuses(), index, penum.StatusEffectType_StatusEffectType_VocalUp)
				if status != nil {
					t.Fatalf("fixture failed to suspend idol %d's buff", index)
				}
			}
			first := result.GetCharts()[0]
			want := int32(2)
			if opponentVocal > 4000 {
				want = 6
			}
			if active := first.GetActivatedSkill(); !active.GetActivated() || active.GetCardIndex() != want {
				t.Fatalf("restored recipient Vocal4000 must determine authority: want=%d got=%v", want, active)
			}
			_, source := liveStatusAt(t, first.GetCardStatuses(), 1, penum.StatusEffectType_StatusEffectType_VocalUp)
			_, recipient := liveStatusAt(t, first.GetCardStatuses(), 2, penum.StatusEffectType_StatusEffectType_VocalUp)
			if source != nil || recipient.GetGrade() != 20 || recipient.GetRemainEffectedChartCount() != 9 {
				t.Fatalf("only recipient owns the transferred anchor, even after losing authority: source=%v recipient=%v", source, recipient)
			}
			// Assignment expires after the first event; the next anchor belongs
			// to the original lane again, so its still-paused buff now restores.
			_, source = liveStatusAt(t, result.GetCharts()[1].GetCardStatuses(), 1, penum.StatusEffectType_StatusEffectType_VocalUp)
			if source.GetGrade() != 20 || source.GetRemainEffectedChartCount() != 9 {
				t.Fatalf("source's migration was consumed or aged by the transferred anchor: %v", source)
			}
		})
	}
}
