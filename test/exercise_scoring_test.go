package test

import (
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	exerciselogic "github.com/vertesan/solis-server/internal/makino/exercise"
	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"google.golang.org/protobuf/proto"
)

func TestExerciseScoringRulesScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	var calibratedMusic string
	for _, id := range []string{"exercise-basic-01", "exercise-basic-02", "exercise-basic-03", "exercise-basic-04", "exercise-basic-05", "exercise-basic-06", "exercise-basic-07"} {
		exercise, err := opmaster.FindExercise(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		quest, err := opmaster.FindQuest(ctx, exercise.GetQuestId())
		if err != nil {
			t.Fatal(err)
		}
		_, coefficient, disableBase, err := exerciselogic.ScoringParameters(id, quest.GetMusicId())
		if err != nil || !disableBase || coefficient <= 0 {
			t.Fatalf("captured lesson %s lost its verified scoring rules: coefficient=%d disableBase=%t err=%v", id, coefficient, disableBase, err)
		}
		calibratedMusic = quest.GetMusicId()
	}
	// Later lessons include an explicit technique-equipment tutorial. The
	// first seven lessons' absent base criticals must not become a global rule.
	exercise, err := opmaster.FindExercise(ctx, "exercise-basic-22")
	if err != nil {
		t.Fatal(err)
	}
	quest, err := opmaster.FindQuest(ctx, exercise.GetQuestId())
	if err != nil {
		t.Fatal(err)
	}
	_, coefficient, disableBase, err := exerciselogic.ScoringParameters(exercise.GetId(), quest.GetMusicId())
	if err != nil || disableBase {
		t.Fatalf("technique tutorial incorrectly disables technique criticals: disableBase=%t err=%v", disableBase, err)
	}
	if _, _, sharedMusicDisablesBase, err := exerciselogic.ScoringParameters(exercise.GetId(), calibratedMusic); err != nil || sharedMusicDisablesBase {
		t.Fatalf("using a calibrated song incorrectly disabled a later lesson's technique: %v", err)
	}
	in := liveEngineFixture(1)
	in.DisableBaseCriticalRate = disableBase
	in.BeatScoreCoefficientPermil = coefficient
	in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_Beat}}
	for _, card := range in.Users[0].UserDeck.Cards {
		card.Technique = 1_000_000
		card.Skills = nil
	}
	result, err := live.Simulate(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.GetCharts()) != 1 || len(result.GetCharts()[0].GetBeats()) != 5 {
		t.Fatal("technique tutorial fixture did not produce five scoring beats")
	}
	for _, beat := range result.GetCharts()[0].GetBeats() {
		if !beat.GetIsCritical() {
			t.Fatal("later-lesson rules ignored guaranteed technique criticals")
		}
	}
}

func TestExerciseDeterministicScoringScenario(t *testing.T) {
	setupTestUser(t)
	in := liveEngineFixture(1)
	in.DisableBaseCriticalRate = true
	in.Chart = nil
	for number := int32(1); number <= 12; number++ {
		in.Chart = append(in.Chart, &pmaster.MusicChartPattern{Number: number, Type: penum.MusicChartType_MusicChartType_Beat})
	}
	for _, card := range in.Users[0].UserDeck.Cards {
		// This would guarantee normal-live criticals. Exercise must suppress
		// the technique term, not change the prepared card's displayed stats.
		card.Technique = 1_000_000
		card.Skills = nil
	}
	run := func() *papi.LiveResult {
		t.Helper()
		result, err := live.Simulate(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := run()
	if !proto.Equal(first, run()) {
		t.Fatal("the same exercise input and seed produced different results")
	}
	values := map[int64]bool{}
	for _, chart := range first.GetCharts() {
		for _, beat := range chart.GetBeats() {
			if beat.GetIsCritical() {
				t.Fatal("exercise technique alone produced a critical")
			}
			values[beat.GetScore()] = true
		}
	}
	if len(values) < 2 {
		t.Fatal("exercise removed the deterministic score jitter observed in captures")
	}

	// The captured criticals occur while CriticalRateUp is present. The mode
	// must not disable these buffs, even though the natural rate is zero.
	in.Targets["target-self"] = &pmaster.SkillTarget{Id: "target-self", Type: penum.SkillTargetType_SkillTargetType_Self}
	in.Triggers["tg-live_start"] = &pmaster.SkillTrigger{Id: "tg-live_start", Type: penum.SkillTriggerType_SkillTriggerType_LiveStart}
	in.Setting.StatusEffectTypeStrengthList = []penum.StatusEffectType{penum.StatusEffectType_StatusEffectType_CriticalRateUp}
	effect := liveStatusDefinition(in, "critical_rate_up", penum.SkillEfficacyType_SkillEfficacyType_CriticalRateUp, 20, 20, 0, 20, "target-self")
	liveStatusSkill(in, "exercise-critical-buff", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, effect)
	in.Users[0].UserDeck.Cards[0].Skills = []*papi.LiveDeckCardSkill{{Index: 1, SkillId: "exercise-critical-buff", SkillLevel: 1, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card}}
	for _, chart := range run().GetCharts() {
		for _, beat := range chart.GetBeats() {
			if beat.GetIsCritical() != (beat.GetCardIndex() == 1) {
				t.Fatal("exercise critical buff was suppressed or leaked to another actor")
			}
		}
	}

	// Normal lives retain the existing technique formula.
	in.DisableBaseCriticalRate = false
	for _, chart := range run().GetCharts() {
		for _, beat := range chart.GetBeats() {
			if !beat.GetIsCritical() {
				t.Fatal("exercise critical rule changed a normal live")
			}
		}
	}
}

func TestExerciseBeatCoefficientScenario(t *testing.T) {
	setupTestUser(t)
	in := liveEngineFixture(1)
	in.DisableBaseCriticalRate = true
	in.BeatScoreCoefficientPermil = 57
	in.Quest.BeatVocalWeightPermil = 600
	in.Quest.BeatDanceWeightPermil = 200
	in.Quest.BeatVisualWeightPermil = 200
	in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_Beat}, {Number: 2, Type: penum.MusicChartType_MusicChartType_Beat}}
	for _, card := range in.Users[0].UserDeck.Cards {
		card.Skills = nil
		card.Vocal, card.Dance, card.Visual = 1009, 1009, 1009
		// Amplify the rounding difference so every roll distinguishes it.
		data := in.Cards[card.Index]
		data.CharacterAdvantagePermil = 1_000_000
		in.Cards[card.Index] = data
	}
	run := func() *papi.LiveResult {
		t.Helper()
		result, err := live.Simulate(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := run()
	// floor(1009*.6) + floor(1009*.2) + floor(1009*.2) = 1007.
	// A single pre-rounded Vocal term must give the identical score stream.
	in.Quest.BeatVocalWeightPermil = 1000
	in.Quest.BeatDanceWeightPermil, in.Quest.BeatVisualWeightPermil = 0, 0
	for _, card := range in.Users[0].UserDeck.Cards {
		card.Vocal = 1007
	}
	for i, chart := range run().GetCharts() {
		for j, beat := range chart.GetBeats() {
			if !proto.Equal(beat, first.GetCharts()[i].GetBeats()[j]) {
				t.Fatal("exercise weighted attributes were not floored individually")
			}
		}
	}
	// Adding beat events does not dilute a calibrated song's per-beat score.
	in.Chart = append(in.Chart, &pmaster.MusicChartPattern{Number: 3, Type: penum.MusicChartType_MusicChartType_Beat}, &pmaster.MusicChartPattern{Number: 4, Type: penum.MusicChartType_MusicChartType_Beat})
	for i, chart := range run().GetCharts()[:2] {
		for j, beat := range chart.GetBeats() {
			if !proto.Equal(beat, first.GetCharts()[i].GetBeats()[j]) {
				t.Fatal("exercise beat coefficient still depends on chart beat count")
			}
		}
	}
	in.BeatScoreCoefficientPermil = 0
	for index, data := range in.Cards {
		data.CharacterAdvantagePermil = 0
		in.Cards[index] = data
	}
	for _, beat := range run().GetCharts()[0].GetBeats() {
		// The existing 8*1007/4 formula resumes when the override is absent.
		if beat.GetScore() < 1913 || beat.GetScore() > 2115 {
			t.Fatal("exercise coefficient changed normal-live default scoring")
		}
	}
	for _, invalid := range []int32{-1, 1001} {
		in.BeatScoreCoefficientPermil = invalid
		if result, err := live.Simulate(context.Background(), in); err == nil || result != nil {
			t.Fatal("invalid beat coefficient accepted")
		}
	}
}
