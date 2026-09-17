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

func TestLiveStatusPreflightScenario(t *testing.T) {
	setupTestUser(t)
	for _, invalid := range []string{"mismatched-target", "unsupported-inactive-power"} {
		in := liveEngineFixture(1)
		in.Targets["target-self"] = &pmaster.SkillTarget{Id: "target-self", Type: penum.SkillTargetType_SkillTargetType_Self}
		in.Triggers["tg-combo_count_higher-999999"] = &pmaster.SkillTrigger{Id: "tg-combo_count_higher-999999", Type: penum.SkillTriggerType_SkillTriggerType_Combo}
		var effectID string
		if invalid == "mismatched-target" {
			effectID = "ef-vocal_up-1-target-all-3"
			in.Efficacies[effectID] = &pmaster.SkillEfficacy{Id: effectID, Type: penum.SkillEfficacyType_SkillEfficacyType_VocalUp, Grade: 1, MaxGrade: 20, SkillTargetId: "target-self"}
		} else {
			// score_up is a real status, but its conditional-power coefficient is
			// not supported. A never-triggered P still needs preflight rejection.
			effectID = "ef-score_get_by_status_effect_type_grade-score_up-1000-chart_dependence"
			in.Efficacies[effectID] = &pmaster.SkillEfficacy{Id: effectID, Type: penum.SkillEfficacyType_SkillEfficacyType_ScoreGetByStatusEffectTypeGrade}
		}
		liveStatusSkill(in, "fixture-invalid-passive", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-combo_count_higher-999999", 0, effectID)
		in.Users[0].UserDeck.Cards[0].Skills = append(in.Users[0].UserDeck.Cards[0].Skills, &papi.LiveDeckCardSkill{Index: 3, SkillId: "fixture-invalid-passive", SkillLevel: 1, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card})
		result, err := live.Simulate(context.Background(), in)
		var unsupported *live.UnsupportedError
		if result != nil || !errors.As(err, &unsupported) {
			t.Fatalf("%s was not rejected before an inactive rule could be skipped: result=%t err=%v", invalid, result != nil, err)
		}
	}
}

func liveStatusDefinition(in *live.Input, name string, typ penum.SkillEfficacyType, grade, cap, limit, duration int32, target string) string {
	id := fmt.Sprintf("ef-%s-%d-%s-%d", name, grade, target, duration)
	in.Efficacies[id] = &pmaster.SkillEfficacy{Id: id, Type: typ, Grade: grade, MaxGrade: cap, LimitBreakMaxGrade: limit, SkillTargetId: target}
	return id
}

func liveStatusSkill(in *live.Input, id string, category penum.SkillCategoryType, trigger string, cooldown int32, effects ...string) {
	level := &pmaster.SkillLevel{Level: 1, ProbabilityPermil: 1000, TriggerId: trigger, CoolTime: cooldown}
	for _, effect := range effects {
		level.SkillDetails = append(level.SkillDetails, &pmaster.SkillDetail{EfficacyId: effect})
	}
	in.Skills[id] = &pmaster.Skill{Id: id, CategoryType: category, Levels: []*pmaster.SkillLevel{level}}
}

func liveStatusAt(t *testing.T, states []*papi.LiveCardStatus, card int32, typ penum.StatusEffectType) (*papi.LiveCardStatus, *papi.LiveCardStatusEffect) {
	t.Helper()
	for _, state := range states {
		if state.GetCardIndex() != card {
			continue
		}
		for _, effect := range state.GetEffects() {
			if effect.GetStatusType() == typ {
				return state, effect
			}
		}
		return state, nil
	}
	t.Fatalf("missing card status %d", card)
	return nil, nil
}

func TestLiveStatusLifecycleScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	in := liveEngineFixture(1)
	in.Targets["target-self"] = &pmaster.SkillTarget{Id: "target-self", Type: penum.SkillTargetType_SkillTargetType_Self}
	in.Triggers["tg-live_start"] = &pmaster.SkillTrigger{Id: "tg-live_start", Type: penum.SkillTriggerType_SkillTriggerType_LiveStart}
	vocal := penum.StatusEffectType_StatusEffectType_VocalUp
	capType := penum.StatusEffectType_StatusEffectType_LimitBreakVocalUp
	in.Setting.StatusEffectTypeStrengthList = []penum.StatusEffectType{vocal}
	capEffect := liveStatusDefinition(in, "limit_break_vocal_up", penum.SkillEfficacyType_SkillEfficacyType_LimitBreakVocalUp, 10, 10, 0, 2, "target-self")
	upEffect := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 15, 20, 30, 3, "target-self")
	liveStatusSkill(in, "fixture-start-cap", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-live_start", 0, capEffect)
	liveStatusSkill(in, "fixture-active-up", penum.SkillCategoryType_SkillCategoryType_Active, "", 0, "ef-score_get-1000-chart_dependence", upEffect)
	for _, card := range in.Users[0].UserDeck.Cards {
		card.Skills = nil
	}
	in.Users[0].UserDeck.Cards[0].Skills = []*papi.LiveDeckCardSkill{
		{Index: 1, SkillId: "fixture-active-up", SkillLevel: 1, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card},
		{Index: 2, SkillId: "fixture-start-cap", SkillLevel: 1, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card},
	}
	in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}, {Number: 2, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1}, {Number: 3, Type: penum.MusicChartType_MusicChartType_Beat}, {Number: 4, Type: penum.MusicChartType_MusicChartType_Beat}}
	before := proto.Clone(in.Users[0])
	result, err := live.Simulate(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, in.Users[0]) {
		t.Fatal("status simulation modified the input unit")
	}
	if len(result.GetActivatedPassiveSkills()) != 1 || !result.GetActivatedPassiveSkills()[0].GetActivated() {
		t.Fatal("start-of-live cap did not activate exactly once")
	}
	_, initialCap := liveStatusAt(t, result.GetPreChart().GetCardStatuses(), 1, capType)
	if initialCap.GetGrade() != 10 || initialCap.GetRemainEffectedChartCount() != 2 {
		t.Fatalf("pre-live cap was mutated by later ticks: %v", initialCap)
	}
	first, firstUp := liveStatusAt(t, result.GetCharts()[0].GetCardStatuses(), 1, vocal)
	if firstUp.GetGrade() != 15 || firstUp.GetRemainEffectedChartCount() != 2 || first.GetVocal() != 1750 {
		t.Fatalf("first buff snapshot drifted: %v", first)
	}
	secondActive, activeUp := liveStatusAt(t, result.GetCharts()[1].GetActivatedSkill().GetCardStatuses(), 1, vocal)
	if activeUp.GetGrade() != 30 || activeUp.GetRemainEffectedChartCount() != 3 || secondActive.GetVocal() != 2500 {
		t.Fatalf("overlapping buffs did not use the temporary cap before expiry: %v", secondActive)
	}
	secondEnd, cappedUp := liveStatusAt(t, result.GetCharts()[1].GetCardStatuses(), 1, vocal)
	if cappedUp.GetGrade() != 20 || cappedUp.GetRemainEffectedChartCount() != 2 || secondEnd.GetVocal() != 2000 {
		t.Fatalf("expired cap was retained in cached attributes: %v", secondEnd)
	}
	_, expiredCap := liveStatusAt(t, result.GetCharts()[1].GetCardStatuses(), 1, capType)
	if expiredCap != nil {
		t.Fatal("temporary cap survived its second event")
	}
	// Repeated instances from one skill retain separate lifetimes, but their
	// display provenance is deduplicated rather than duplicated or discarded.
	if len(cappedUp.GetStatusReasons()) != 1 || cappedUp.GetStatusReasons()[0].GetCardIndex() != 1 || cappedUp.GetStatusReasons()[0].GetSkillIndex() != 1 {
		t.Fatalf("incorrect repeated-source provenance: %v", cappedUp.GetStatusReasons())
	}
	third, thirdUp := liveStatusAt(t, result.GetCharts()[2].GetCardStatuses(), 1, vocal)
	if thirdUp.GetGrade() != 15 || thirdUp.GetRemainEffectedChartCount() != 1 || third.GetVocal() != 1750 {
		t.Fatalf("oldest instance expiry destroyed or retained the wrong buff: %v", third)
	}
	last, lastUp := liveStatusAt(t, result.GetCharts()[3].GetCardStatuses(), 1, vocal)
	if lastUp != nil || last.GetVocal() != 1000 || len(last.GetEffects()) != 0 {
		t.Fatalf("expired status instances did not cleanly disappear: %v", last)
	}
	for _, chart := range result.GetCharts() {
		for _, skill := range chart.GetCardStatuses()[0].GetSkillStatuses() {
			if skill.GetCoolTime() != 0 {
				t.Fatal("zero-CT skills acquired cooldown")
			}
		}
	}

	// Two native and two photo passives alternate on cooldown. A successful
	// early activation owns that category's budget through the late phase.
	quota := liveEngineFixture(1)
	quota.Targets["target-self"] = &pmaster.SkillTarget{Id: "target-self", Type: penum.SkillTargetType_SkillTargetType_Self}
	quota.Setting.StatusEffectTypeStrengthList = []penum.StatusEffectType{vocal, penum.StatusEffectType_StatusEffectType_DanceUp}
	nativeEffect := liveStatusDefinition(quota, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 1, 20, 0, 2, "target-self")
	photoEffect := liveStatusDefinition(quota, "dance_up", penum.SkillEfficacyType_SkillEfficacyType_DanceUp, 1, 20, 0, 2, "target-self")
	for _, card := range quota.Users[0].UserDeck.Cards {
		card.Skills = nil
	}
	for _, definition := range []struct {
		index      int32
		possession penum.SkillPossessionType
		id, effect string
	}{
		{1, penum.SkillPossessionType_SkillPossessionType_Card, "fixture-native-one", nativeEffect}, {2, penum.SkillPossessionType_SkillPossessionType_Card, "fixture-native-two", nativeEffect},
		{1001, penum.SkillPossessionType_SkillPossessionType_Photo, "fixture-photo-one", photoEffect}, {1002, penum.SkillPossessionType_SkillPossessionType_Photo, "fixture-photo-two", photoEffect},
	} {
		liveStatusSkill(quota, definition.id, penum.SkillCategoryType_SkillCategoryType_Passive, "", 2, definition.effect)
		quota.Users[0].UserDeck.Cards[0].Skills = append(quota.Users[0].UserDeck.Cards[0].Skills, &papi.LiveDeckCardSkill{Index: definition.index, SkillId: definition.id, SkillLevel: 1, PossessionType: definition.possession})
	}
	quota.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_Beat}, {Number: 2, Type: penum.MusicChartType_MusicChartType_Beat}, {Number: 3, Type: penum.MusicChartType_MusicChartType_Beat}, {Number: 4, Type: penum.MusicChartType_MusicChartType_Beat}}
	quotaResult, err := live.Simulate(ctx, quota)
	if err != nil {
		t.Fatal(err)
	}
	for i, chart := range quotaResult.GetCharts() {
		var activated []int32
		for _, skill := range chart.GetActivatedPassiveSkills() {
			if skill.GetActivated() {
				activated = append(activated, skill.GetSkillIndex())
				if skill.GetOrder() >= chart.GetBeats()[0].GetOrder() {
					t.Fatal("quota fixture unexpectedly activated a second passive late")
				}
			}
		}
		wantNative := int32(i%2 + 1)
		if len(activated) != 2 || activated[0] != wantNative || activated[1] != 1000+wantNative {
			t.Fatalf("event %d native/photo budget or cooldown mismatch: %v", i+1, activated)
		}
		state, up := liveStatusAt(t, chart.GetCardStatuses(), 1, vocal)
		if up.GetGrade() != 1 || up.GetRemainEffectedChartCount() != 1 || len(up.GetStatusReasons()) != 1 || up.GetStatusReasons()[0].GetSkillIndex() != wantNative {
			t.Fatalf("event %d retained expired source: %v", i+1, state)
		}
	}
}

func liveHeavyEffectsFixture(events int) *live.Input {
	in := liveEngineFixture(2)
	in.Targets["target-self"] = &pmaster.SkillTarget{Id: "target-self", Type: penum.SkillTargetType_SkillTargetType_Self}
	in.Targets["target-all"] = &pmaster.SkillTarget{Id: "target-all", Type: penum.SkillTargetType_SkillTargetType_All}
	in.Targets["target-opponent_all"] = &pmaster.SkillTarget{Id: "target-opponent_all", Type: penum.SkillTargetType_SkillTargetType_OpponentAll, IsOpponent: true}
	in.Triggers["tg-always"] = &pmaster.SkillTrigger{Id: "tg-always", Type: penum.SkillTriggerType_SkillTriggerType_Beat}
	in.Setting.StatusEffectTypeStrengthList = []penum.StatusEffectType{penum.StatusEffectType_StatusEffectType_DanceUp, penum.StatusEffectType_StatusEffectType_VocalUp, penum.StatusEffectType_StatusEffectType_VisualUp}
	in.Setting.StatusEffectTypeWeaknessDownList = []penum.StatusEffectType{penum.StatusEffectType_StatusEffectType_VisualDown}
	nativeSelf := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 3, 20, 0, 20, "target-self")
	nativeAll := liveStatusDefinition(in, "dance_up", penum.SkillEfficacyType_SkillEfficacyType_DanceUp, 2, 20, 0, 16, "target-all")
	photoSelf := liveStatusDefinition(in, "visual_up", penum.SkillEfficacyType_SkillEfficacyType_VisualUp, 4, 20, 0, 10, "target-self")
	photoOpponents := liveStatusDefinition(in, "visual_down", penum.SkillEfficacyType_SkillEfficacyType_VisualDown, 2, 20, 0, 18, "target-opponent_all")
	liveStatusSkill(in, "fixture-heavy-native", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-always", 3, nativeSelf, nativeAll)
	liveStatusSkill(in, "fixture-heavy-photo", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-always", 3, photoSelf, photoOpponents)
	in.Skills["fixture-skill-2"].Levels[0].SkillDetails = append(in.Skills["fixture-skill-2"].Levels[0].SkillDetails, &pmaster.SkillDetail{EfficacyId: nativeSelf})
	for _, user := range in.Users {
		for _, card := range user.UserDeck.Cards {
			card.Stamina = 20000
			card.Skills = append(card.Skills, &papi.LiveDeckCardSkill{Index: 3, SkillId: "fixture-heavy-native", SkillLevel: 1, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card}, &papi.LiveDeckCardSkill{Index: 1001, SkillId: "fixture-heavy-photo", SkillLevel: 1, PossessionType: penum.SkillPossessionType_SkillPossessionType_Photo})
		}
	}
	in.Chart = make([]*pmaster.MusicChartPattern, events)
	for i := range in.Chart {
		in.Chart[i] = &pmaster.MusicChartPattern{Number: int32(i + 1), Type: penum.MusicChartType_MusicChartType_Beat}
		if i%3 == 2 {
			in.Chart[i].Type = penum.MusicChartType_MusicChartType_ActiveSkill
			in.Chart[i].Position = int32((i/3)%5 + 1)
		}
		if i%30 == 29 {
			in.Chart[i].Type = penum.MusicChartType_MusicChartType_SpecialSkill
		}
	}
	return in
}

func BenchmarkLiveEffects(b *testing.B) {
	for _, events := range []int{150, 1500} {
		b.Run(fmt.Sprintf("battle_%d_events", events), func(b *testing.B) {
			in := liveHeavyEffectsFixture(events)
			var result *papi.LiveResult
			var err error
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				result, err = live.Simulate(context.Background(), in)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(proto.Size(result)), "protobuf-bytes/live")
		})
	}
}
