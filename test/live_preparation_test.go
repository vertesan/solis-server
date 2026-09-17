package test

import (
	"math"
	"reflect"
	"testing"

	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

// Pure preparation cases still live under test/, using the shared test harness.
// No user documents are created or changed by these arithmetic scenarios.
func TestLiveNPCPreparationScenario(t *testing.T) {
	setting := &pmaster.Setting{CardDefaultMental: 100, CardDefaultTechnique: 100}
	for _, tc := range []struct {
		difficulty, threshold               int32
		primary, stamina, mental, technique int64
	}{{33, 58, 350, 0, 130, 130}, {66, 200, 4448, 132, 231, 245}, {122, 1433, 30161, 961, 1290, 966}, {200, 3054, 105311, 3385, 2749, 3075}, {320, 4365, 310350, 10000, 7830, 8830}, {325, 4444, 321151, 10348, 8098, 9133}, {337, 3937, 347074, 11184, 8742, 9860}} {
		q := &pmaster.Quest{DifficultyLevel: tc.difficulty, MentalThreshold: tc.threshold, MaxCapacity: 80000}
		got, err := live.NPCBalance(q, setting)
		if err != nil {
			t.Fatal(err)
		}
		if got.Version != "npc-balance-v1" || got.Primary != tc.primary || got.Stamina != tc.stamina || got.Mental != tc.mental || got.Technique != tc.technique {
			t.Fatalf("difficulty %d: unexpected bonus %+v", tc.difficulty, got)
		}
		again, err := live.NPCBalance(q, setting)
		if err != nil || got != again {
			t.Fatalf("balance is not deterministic: %+v / %+v: %v", got, again, err)
		}
	}
	var previous live.NPCBonuses
	for d := int32(319); d <= 321; d++ {
		got, err := live.NPCBalance(&pmaster.Quest{DifficultyLevel: d, MentalThreshold: 100, MaxCapacity: 100}, setting)
		if err != nil {
			t.Fatal(err)
		}
		if d > 319 && (got.Primary < previous.Primary || got.Stamina < previous.Stamina || got.Mental < previous.Mental || got.Technique < previous.Technique) {
			t.Fatal("NPC curve decreases at splice")
		}
		previous = got
	}
	if _, err := live.NPCBalance(&pmaster.Quest{DifficultyLevel: 0, MentalThreshold: 1, MaxCapacity: 1}, setting); err == nil {
		t.Fatal("zero difficulty accepted")
	}
	if _, err := live.NPCBalance(&pmaster.Quest{DifficultyLevel: math.MaxInt32, MentalThreshold: math.MaxInt32, MaxCapacity: 1}, setting); err != nil {
		t.Fatalf("int64-safe late difficulty rejected: %v", err)
	}
	// The roster is returned by value; mutating a request's copy cannot poison
	// the next player's NPC configuration.
	roster, err := live.NPCRoster("qt-tower-975")
	if err != nil {
		t.Fatal(err)
	}
	if len(roster.GetCardInfos()) != 5 {
		t.Fatal("NPC roster incomplete")
	}
	id := roster.CardInfos[0].GetCardId()
	roster.CardInfos[0].CardId = "mutated"
	next, err := live.NPCRoster("qt-tower-975")
	if err != nil || next.CardInfos[0].GetCardId() != id {
		t.Fatal("NPC roster storage was mutated")
	}
	if _, err := live.NPCRoster("not-a-quest"); err == nil {
		t.Fatal("missing NPC roster accepted")
	}
}

func TestLiveAudienceAllocationScenario(t *testing.T) {
	setting := &pmaster.Setting{LiveLightFanAmountWeightPermil: 50, LiveMiddleFanAmountWeightPermil: 200, LiveHeavyFanAmountWeightPermil: 500, LiveMinimumGuaranteedAudienceAmount: 10}
	candidates := []int64{52599, 52588, 52620, 52620, 52689}
	cards := make([]live.CardData, len(candidates))
	for i, n := range candidates {
		cards[i].LightFans = n * 20
	}
	for _, tc := range []struct {
		capacity int32
		expected []int32
	}{{80000, []int32{15993, 15990, 16000, 16000, 16017}}, {70000, []int32{13994, 13991, 14000, 14000, 14015}}} {
		got, err := live.AllocateAudience(cards, tc.capacity, setting)
		if err != nil || !reflect.DeepEqual(got, tc.expected) {
			t.Fatalf("capacity %d allocation %v, want %v: %v", tc.capacity, got, tc.expected, err)
		}
	}
	// Every zero-fan actor starts from the candidate minimum. Overbooking
	// decrements the largest current allocation, choosing latest-index ties.
	zero := make([]live.CardData, 5)
	got, err := live.AllocateAudience(zero, 12, setting)
	if err != nil || !reflect.DeepEqual(got, []int32{3, 3, 2, 2, 2}) {
		t.Fatalf("minimum/tie allocation: %v, %v", got, err)
	}
	got, err = live.AllocateAudience(make([]live.CardData, 10), 100, setting)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range got {
		if n != 10 {
			t.Fatal("battle sides do not share the same allocation rules")
		}
	}
	zero[0].LightFans = -1
	if _, err := live.AllocateAudience(zero, 12, setting); err == nil {
		t.Fatal("negative fans accepted")
	}
	zero[0].LightFans = math.MaxInt64
	if _, err := live.AllocateAudience(zero, 12, setting); err == nil {
		t.Fatal("overflowing fans accepted")
	}
	if _, err := live.AllocateAudience(cards, 0, setting); err == nil {
		t.Fatal("zero-capacity venue accepted")
	}
}
