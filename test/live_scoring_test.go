package test

import (
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

func TestLiveScoringTablesScenario(t *testing.T) {
	setupTestUser(t)
	in := liveEngineFixture(1)
	in.ComboAdvantages = []*pmaster.ComboAdvantage{{ComboCount: 10, AdvantagePermil: 1050}}
	in.Chart = nil
	for number := int32(1); number <= 11; number++ {
		in.Chart = append(in.Chart, &pmaster.MusicChartPattern{Number: number, Type: penum.MusicChartType_MusicChartType_Beat})
	}
	result, err := live.Simulate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	for i, chart := range result.GetCharts() {
		for _, beat := range chart.GetBeats() {
			// 8 * Vocal1000 / 11 normal beats, before jitter and combo.
			lower, upper := int64(690), int64(763)
			if i == 10 {
				lower, upper = 725, 801
			}
			if beat.GetIsCritical() || beat.GetScore() < lower || beat.GetScore() > upper {
				t.Fatalf("beat %d score %d outside %d..%d; combo master factor must not be added twice", i+1, beat.GetScore(), lower, upper)
			}
		}
	}
	// Force criticals and add ten percentage points to the critical multiplier.
	// One normal beat: 8000 base × (1.5 + .1) × jitter = 12160..13440,
	// not an extra 1.1 multiplier.
	in.Chart = in.Chart[:1]
	for _, card := range in.Users[0].UserDeck.Cards {
		card.Technique = 1_000_000
		data := in.Cards[card.Index]
		data.Bonuses = map[penum.PhotoAbilityType]int64{penum.PhotoAbilityType_PhotoAbilityType_CriticalScoreMultiply: 100}
		in.Cards[card.Index] = data
	}
	critical, err := live.Simulate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	for _, beat := range critical.GetCharts()[0].GetBeats() {
		if !beat.GetIsCritical() || beat.GetScore() < 12160 || beat.GetScore() > 13440 {
			t.Fatalf("critical multiplier: %v", beat)
		}
	}
}

func TestLiveSkillScoringScenario(t *testing.T) {
	setupTestUser(t)
	in := liveEngineFixture(1)
	in.Quest.ActiveSkillWeightPermil = 2000
	in.Quest.SpecialSkillWeightPermil = 500
	in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}, {Number: 2, Type: penum.MusicChartType_MusicChartType_SpecialSkill, Position: 1}}
	data := in.Cards[1]
	data.Bonuses = map[penum.PhotoAbilityType]int64{penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreAdd: 31, penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreMultiply: 200, penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreAdd: 17, penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreMultiply: 300}
	in.Cards[1] = data
	result, err := live.Simulate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	expected := [][2]int64{{2311, 2551}, {634, 699}}
	for i, chart := range result.GetCharts() {
		skill := chart.GetActivatedSkill()
		if !skill.GetActivated() || skill.GetScore() < expected[i][0] || skill.GetScore() > expected[i][1] {
			t.Fatalf("skill category %d stage/photo coefficients: score=%d activated=%v", i, skill.GetScore(), skill.GetActivated())
		}
	}
}

func TestLiveLowFanScoringScenario(t *testing.T) {
	setupTestUser(t)
	in := liveEngineFixture(1)
	in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}}
	const id = "ef-score_get_by_less_fan_amount-1000-chart_dependence"
	in.Efficacies[id] = &pmaster.SkillEfficacy{Id: id, Type: penum.SkillEfficacyType_SkillEfficacyType_ScoreGetByLessFanAmount}
	in.Skills["fixture-skill-2"].Levels[0].SkillDetails[0].EfficacyId = id
	run := func(fans int64) int64 {
		t.Helper()
		data := in.Cards[1]
		data.LightFans, data.MiddleFans, data.HeavyFans = fans, 0, 0
		in.Cards[1] = data
		result, err := live.Simulate(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		skill := result.GetCharts()[0].GetActivatedSkill()
		if !skill.GetActivated() {
			t.Fatal("low-fan skill did not activate")
		}
		return skill.GetScore()
	}
	// Keep this prepared fixture's audience fixed to isolate the power curve.
	// Same seed => identical jitter, so only conditional power may differ.
	baseline := run(int64(in.Quest.MaxCapacity) * 2)
	if over := run(int64(in.Quest.MaxCapacity) * 20); over != baseline {
		t.Fatal("low-fan bonus did not clamp at its target")
	}
	low, middle := run(0), run(int64(in.Quest.MaxCapacity))
	if low < baseline*18/10 || low > baseline*18/10+2 {
		t.Fatalf("zero-fan multiplier is not 1.8: base=%d low=%d", baseline, low)
	}
	if middle < baseline*12/10 || middle > baseline*12/10+2 {
		t.Fatalf("half-target multiplier is not 1.2: base=%d middle=%d", baseline, middle)
	}
}
