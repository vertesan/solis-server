package test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"google.golang.org/protobuf/proto"
)

func liveEngineFixture(sides int) *live.Input {
	in := &live.Input{
		Quest: &pmaster.Quest{Id: "fixture-live", DifficultyLevel: 10, MentalThreshold: 10, MaxCapacity: int32(500 * sides), ClearScore: 1,
			Position1AttributeType: penum.AttributeType_AttributeType_Vocal, Position2AttributeType: penum.AttributeType_AttributeType_Vocal, Position3AttributeType: penum.AttributeType_AttributeType_Vocal, Position4AttributeType: penum.AttributeType_AttributeType_Vocal, Position5AttributeType: penum.AttributeType_AttributeType_Vocal,
			ActiveSkillWeightPermil: 1000, SpecialSkillWeightPermil: 1000, SkillStaminaWeightPermil: 1000,
			BeatVocalWeightPermil: 1000,
		},
		Setting:            &pmaster.Setting{LiveMaxParameterBonusPermil: 1000000},
		Pressure:           &pmaster.QuestPressure{WeightPermil: 1000},
		AudienceAdvantages: []*pmaster.QuestAudienceAdvantage{{AudienceAmount: 0, AdvantagePermil: 1000}},
		ComboAdvantages:    []*pmaster.ComboAdvantage{{ComboCount: 0, AdvantagePermil: 1000}},
		StaminaConsumption: []*pmaster.QuestStaminaConsumption{{RemainStaminaRatioPercent: 100, SkillTargetParameterWeightPermil: 1000}},
		Cards:              map[int32]live.CardData{}, Skills: map[string]*pmaster.Skill{}, Efficacies: map[string]*pmaster.SkillEfficacy{},
		Targets: map[string]*pmaster.SkillTarget{}, Triggers: map[string]*pmaster.SkillTrigger{}, Seed: 123456,
	}
	const effectID = "ef-score_get-1000-chart_dependence"
	in.Efficacies[effectID] = &pmaster.SkillEfficacy{Id: effectID, Type: penum.SkillEfficacyType_SkillEfficacyType_ScoreGet}
	for _, definition := range []struct {
		id       string
		category penum.SkillCategoryType
	}{
		{"fixture-skill-2", penum.SkillCategoryType_SkillCategoryType_Active},
		{"fixture-skill-1", penum.SkillCategoryType_SkillCategoryType_Special},
	} {
		in.Skills[definition.id] = &pmaster.Skill{Id: definition.id, CategoryType: definition.category, Levels: []*pmaster.SkillLevel{{Level: 1, Stamina: 10, CoolTime: 3, ProbabilityPermil: 1000, SkillDetails: []*pmaster.SkillDetail{{EfficacyId: effectID}}}}}
	}
	for side := 0; side < sides; side++ {
		user := &papi.LiveUserInfo{Index: int32(side + 1), UserId: testConfig.TestUserPublicId, IsNpc: side == 1, UserDeck: &papi.LiveUserDeck{DeckName: "engine fixture"}}
		if side == 1 {
			user.UserId = ""
		}
		for lane := 1; lane <= 5; lane++ {
			index := int32(side*5 + lane)
			strength := int64(1000)
			if side == 1 {
				strength = 500
			}
			user.UserDeck.Cards = append(user.UserDeck.Cards, &papi.LiveDeckCard{
				Index: index, CardId: fmt.Sprintf("fixture-card-%d", index), Vocal: strength, Dance: strength, Visual: strength,
				Stamina: 500, Mental: 1000, AudienceAmount: 100,
				Skills: []*papi.LiveDeckCardSkill{{Index: 1, SkillId: "fixture-skill-2", SkillLevel: 1, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card}, {Index: 2, SkillId: "fixture-skill-1", SkillLevel: 1, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card}},
			})
			in.Cards[index] = live.CardData{CharacterID: fmt.Sprintf("fixture-character-%d", index), LightFans: 2000}
		}
		in.Users = append(in.Users, user)
	}
	in.Chart = []*pmaster.MusicChartPattern{
		{Number: 1, Type: penum.MusicChartType_MusicChartType_Beat}, {Number: 10, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}, {Number: 11, Type: penum.MusicChartType_MusicChartType_Unknown},
		{Number: 20, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}, {Number: 21, Type: penum.MusicChartType_MusicChartType_Beat}, {Number: 22, Type: penum.MusicChartType_MusicChartType_Beat},
		{Number: 40, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}, {Number: 50, Type: penum.MusicChartType_MusicChartType_SpecialSkill, Position: 3},
	}
	return in
}

func TestLiveEngineDeterministicFlow(t *testing.T) {
	setupTestUser(t)
	in := liveEngineFixture(1)
	beforeUser := proto.Clone(in.Users[0])
	beforeQuest := proto.Clone(in.Quest)
	beforeSkill := proto.Clone(in.Skills["fixture-skill-2"])
	result, err := live.Simulate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.GetCharts()) != 7 || !result.GetCleared() {
		t.Fatalf("wrong chart filtering/clear: charts=%d cleared=%v", len(result.GetCharts()), result.GetCleared())
	}
	if !proto.Equal(beforeUser, in.Users[0]) || !proto.Equal(beforeQuest, in.Quest) || !proto.Equal(beforeSkill, in.Skills["fixture-skill-2"]) {
		t.Fatal("simulation mutated immutable input")
	}
	again, err := live.Simulate(context.Background(), in)
	if err != nil || !proto.Equal(result, again) {
		t.Fatalf("same input/seed is not deterministic: %v", err)
	}
	byNumber := map[int32]*papi.LiveChart{}
	for _, chart := range result.GetCharts() {
		byNumber[chart.GetNumber()] = chart
	}
	if !byNumber[10].GetActivatedSkill().GetActivated() || byNumber[20].GetActivatedSkill().GetActivated() || !byNumber[40].GetActivatedSkill().GetActivated() || !byNumber[50].GetActivatedSkill().GetActivated() {
		t.Fatal("active cooldown or special-skill scheduling is incorrect")
	}
	if result.GetPreChart().GetCardStatuses()[0].GetStamina() != 500 || byNumber[10].GetCardStatuses()[0].GetStamina() != 490 || byNumber[40].GetCardStatuses()[0].GetStamina() != 480 {
		t.Fatal("snapshots were aliased or skill stamina was not consumed once")
	}
	var beatAndSkillScore, cardScore int64
	var previousOrder int32
	for _, chart := range result.GetCharts() {
		for _, beat := range chart.GetBeats() {
			beatAndSkillScore += beat.GetScore()
			if beat.GetOrder() <= previousOrder {
				t.Fatal("trace event order is not strictly increasing")
			}
			previousOrder = beat.GetOrder()
		}
		if skill := chart.GetActivatedSkill(); skill != nil {
			beatAndSkillScore += skill.GetScore()
			if skill.GetOrder() <= previousOrder {
				t.Fatal("skill order is not after previous event")
			}
			previousOrder = skill.GetOrder()
		}
	}
	for _, card := range result.GetUserInfos()[0].GetUserResult().GetCardResults() {
		cardScore += card.GetTotalScore()
	}
	if total := result.GetUserInfos()[0].GetUserResult().GetScore(); total != beatAndSkillScore || total != cardScore {
		t.Fatal("live/card/trace score totals disagree")
	}
	if result.GetUserInfos()[0].GetUserResult().GetHitCount() != 3 {
		t.Fatal("hit count must count the three successful A/SP activations")
	}
	// Reusing the input with a new seed changes stochastic scores, not inputs or
	// the deterministic eligibility schedule.
	in.Seed++
	changed, err := live.Simulate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if proto.Equal(result, changed) {
		t.Fatal("different seeds produced identical complete stochastic traces")
	}
}

func TestLiveEngineBattleAndInvalidFlow(t *testing.T) {
	setupTestUser(t)
	in := liveEngineFixture(2)
	// Remove both SP skills. Winning/losing an earlier A opportunity preserves
	// the loser's combo; the both-no-SP opportunity breaks both sides.
	for _, u := range in.Users {
		for _, c := range u.UserDeck.Cards {
			c.Skills = c.Skills[:1]
		}
	}
	in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_Beat}, {Number: 2, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}, {Number: 3, Type: penum.MusicChartType_MusicChartType_SpecialSkill, Position: 3}}
	result, err := live.Simulate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.GetUserInfos()) != 2 || len(result.GetCharts()[0].GetBeats()) != 10 {
		t.Fatal("battle did not produce ten-lane shared trace")
	}
	active := result.GetCharts()[1].GetActivatedSkill()
	if !active.GetActivated() || active.GetCardIndex() != 1 || len(active.GetFailures()) != 1 || active.GetFailures()[0].GetType() != penum.SkillFailureType_SkillFailureType_OpponentActivation {
		t.Fatalf("unexpected battle authority: %v", active)
	}
	statuses := result.GetCharts()[1].GetUserStatuses()
	if statuses[0].GetCurrentComboCount() != 2 || statuses[1].GetCurrentComboCount() != 1 {
		t.Fatal("losing authority broke the opposing combo")
	}
	noWinner := result.GetCharts()[2]
	if noWinner.GetActivatedSkill().GetActivated() || noWinner.GetActivatedSkill().GetCardIndex() != 0 || !noWinner.GetActivatedSkill().GetIsComboBreak() || len(noWinner.GetActivatedSkill().GetFailures()) != 2 {
		t.Fatal("both-no-SP event lost the no-winner/failure evidence")
	}
	for _, s := range noWinner.GetUserStatuses() {
		if s.GetCurrentComboCount() != 0 {
			t.Fatal("both-no-SP event did not break both combos")
		}
	}
	bad := liveEngineFixture(1)
	bad.Users[0].UserDeck.Cards[0].Skills[0].SkillId = "unimplemented-skill"
	if r, err := live.Simulate(context.Background(), bad); r != nil || err == nil {
		t.Fatal("missing skill silently skipped")
	} else {
		var unsupported *live.UnsupportedError
		if !errors.As(err, &unsupported) {
			t.Fatalf("missing definition did not identify unsupported rule: %v", err)
		}
	}
	bad = liveEngineFixture(1)
	bad.Chart = append(bad.Chart, &pmaster.MusicChartPattern{Number: 1, Type: penum.MusicChartType_MusicChartType_Beat})
	if r, err := live.Simulate(context.Background(), bad); r != nil || err == nil {
		t.Fatal("duplicate chart number accepted")
	}
	bad = liveEngineFixture(1)
	bad.Users[0].UserDeck.Cards[0].AudienceAmount = 501
	if r, err := live.Simulate(context.Background(), bad); r != nil || err == nil {
		t.Fatal("over-capacity audience accepted")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if r, err := live.Simulate(cancelled, liveEngineFixture(1)); r != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled live was simulated: %v", err)
	}
}

func BenchmarkLiveEngine(b *testing.B) {
	for _, events := range []int{150, 1500} {
		b.Run(fmt.Sprintf("battle_%d_events", events), func(b *testing.B) {
			in := liveEngineFixture(2)
			in.Chart = make([]*pmaster.MusicChartPattern, events)
			for i := range in.Chart {
				in.Chart[i] = &pmaster.MusicChartPattern{Number: int32(i + 1), Type: penum.MusicChartType_MusicChartType_Beat}
				if i%10 == 9 {
					in.Chart[i].Type = penum.MusicChartType_MusicChartType_ActiveSkill
					in.Chart[i].Position = int32(i%5 + 1)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := live.Simulate(context.Background(), in); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
