package test

import (
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"google.golang.org/protobuf/proto"
)

// Preparation resolves character advantages for both battle sides without
// changing attributes, authority, or the fixed part of score. No user state is
// mutated; the scenario still runs under the shared integration-test harness.
func TestLiveCharacterAdvantagePreparationScenario(t *testing.T) {
	advantage := &pmaster.QuestCharacterAdvantage{CharacterIds: []string{"matching"}, CharacterGroupId: "group", AdvantagePermil: 1250}
	for _, tc := range []struct {
		name string
		data live.CardData
		want int32
	}{
		{"character", live.CardData{CharacterID: "matching"}, 1250},
		{"group", live.CardData{CharacterID: "other", GroupID: "group"}, 1250},
		{"both-not-double", live.CardData{CharacterID: "matching", GroupID: "group"}, 1250},
		{"unmatched", live.CardData{CharacterID: "other", GroupID: "other"}, 1000},
		{"empty", live.CardData{}, 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := live.ResolveCharacterAdvantage(tc.data, advantage)
			if err != nil || got != tc.want {
				t.Fatalf("character advantage=%d want%d: %v", got, tc.want, err)
			}
		})
	}
	if got, err := live.ResolveCharacterAdvantage(live.CardData{CharacterID: "matching"}, nil); err != nil || got != 1000 {
		t.Fatalf("absent character advantage=%d: %v", got, err)
	}
	for _, invalid := range []int32{0, -1} {
		advantage.AdvantagePermil = invalid
		if _, err := live.ResolveCharacterAdvantage(live.CardData{CharacterID: "matching"}, advantage); err == nil {
			t.Fatal("invalid matching character score multiplier accepted")
		}
	}

	in := liveEngineFixture(2)
	in.Chart = []*pmaster.MusicChartPattern{
		{Number: 1, Type: penum.MusicChartType_MusicChartType_Beat},
		{Number: 2, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1},
	}
	data := in.Cards[6]
	data.Bonuses = map[penum.PhotoAbilityType]int64{penum.PhotoAbilityType_PhotoAbilityType_BeatScoreAdd: 31}
	in.Cards[6] = data
	before := proto.Clone(in.Users[1].GetUserDeck()).(*papi.LiveUserDeck)
	baseline, err := live.Simulate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	advantage = &pmaster.QuestCharacterAdvantage{CharacterIds: []string{data.CharacterID}, AdvantagePermil: 3000}
	for index, card := range in.Cards {
		card.CharacterAdvantagePermil, err = live.ResolveCharacterAdvantage(card, advantage)
		if err != nil {
			t.Fatal(err)
		}
		in.Cards[index] = card
	}
	boosted, err := live.Simulate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, in.Users[1].GetUserDeck()) || !proto.Equal(baseline.GetUserInfos()[1].GetUserDeck(), boosted.GetUserInfos()[1].GetUserDeck()) {
		t.Fatal("score-only character advantage changed NPC prepared attributes")
	}
	for _, result := range []*papi.LiveResult{baseline, boosted} {
		activation := result.GetCharts()[1].GetActivatedSkill()
		if !activation.GetActivated() || activation.GetCardIndex() != 1 {
			t.Fatal("score-only advantage changed battle authority", activation)
		}
	}
	for i, beat := range baseline.GetCharts()[0].GetBeats() {
		got := boosted.GetCharts()[0].GetBeats()[i]
		if got.GetCardIndex() != beat.GetCardIndex() {
			t.Fatal("character advantage reordered beat recipients")
		}
		if beat.GetCardIndex() != 6 {
			if got.GetScore() != beat.GetScore() {
				t.Fatal("unmatched actor received character score advantage")
			}
			continue
		}
		want := (beat.GetScore()-31)*3 + 31
		// Both simulations share their jitter; only the final floor can differ
		// by at most two points when multiplying the pre-rounded score by 3.
		if got.GetScore() < want || got.GetScore() > want+2 {
			t.Fatalf("score multiplier/fixed addition composition: base%d boosted%d want%d..%d", beat.GetScore(), got.GetScore(), want, want+2)
		}
	}
}
