package test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

// Official lane numbers run 4, 2, 1, 3, 5 from left to right. Keep geometry
// separate from target/activation precedence, which follows numeric order.
func TestLiveRound6LaneScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	t.Run("skill targets use official geometry and numeric priority", func(t *testing.T) {
		for _, tc := range []struct {
			name, target string
			kind         penum.SkillTargetType
			source       int
			opponent     bool
			strongLane   int
			want         []int32
		}{
			{"center", "target-center", penum.SkillTargetType_SkillTargetType_Center, 5, false, 0, []int32{1}},
			{"opponent center", "target-opponent_center", penum.SkillTargetType_SkillTargetType_OpponentCenter, 5, true, 0, []int32{6}},
			{"center neighbors", "target-neighbor", penum.SkillTargetType_SkillTargetType_Neighbor, 1, false, 0, []int32{2, 3}},
			{"left inner neighbors", "target-neighbor", penum.SkillTargetType_SkillTargetType_Neighbor, 2, false, 0, []int32{1, 4}},
			{"right inner neighbors", "target-neighbor", penum.SkillTargetType_SkillTargetType_Neighbor, 3, false, 0, []int32{1, 5}},
			{"left edge neighbor", "target-neighbor", penum.SkillTargetType_SkillTargetType_Neighbor, 4, false, 0, []int32{2}},
			{"right edge neighbor", "target-neighbor", penum.SkillTargetType_SkillTargetType_Neighbor, 5, false, 0, []int32{3}},
			{"excess scorers", "target-character_type-1-2", penum.SkillTargetType_SkillTargetType_CardType, 5, false, 0, []int32{1, 2}},
			{"excess attribute", "target-vocal-2", penum.SkillTargetType_SkillTargetType_Vocal, 5, false, 0, []int32{1, 2}},
			{"equal ranked attributes", "target-vocal_higher-2", penum.SkillTargetType_SkillTargetType_VocalHigher, 5, false, 0, []int32{1, 2}},
			{"ranked attributes before lanes", "target-vocal_higher-2", penum.SkillTargetType_SkillTargetType_VocalHigher, 5, false, 4, []int32{4, 1}},
			{"opponent excess scorers", "target-opponent_character_type-1-2", penum.SkillTargetType_SkillTargetType_OpponentCardType, 5, true, 0, []int32{6, 7}},
			{"opponent same position", "target-opponent_same_position", penum.SkillTargetType_SkillTargetType_OpponentSamePosition, 4, true, 0, []int32{9}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				// Multiple seeds must not change excess-filter recipient selection.
				for seed := uint64(1); seed <= 3; seed++ {
					in := liveEngineFixture(2)
					in.Seed = seed
					for _, user := range in.Users {
						for _, card := range user.UserDeck.Cards {
							card.Skills = nil
							data := in.Cards[card.Index]
							data.Type, data.Attribute = penum.CardType_CardType_Appeal, penum.AttributeType_AttributeType_Vocal
							in.Cards[card.Index] = data
						}
					}
					if tc.strongLane != 0 {
						in.Users[0].UserDeck.Cards[tc.strongLane-1].Vocal = 2000
					}
					in.Targets[tc.target] = &pmaster.SkillTarget{Id: tc.target, Type: tc.kind, IsOpponent: tc.opponent}
					effect := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 1, 20, 30, 10, tc.target)
					liveStatusSkill(in, "round6-target", penum.SkillCategoryType_SkillCategoryType_Active, "", 0, effect)
					round5AddSkill(in, tc.source, 1, "round6-target")
					in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: int32(tc.source)}}
					result, err := live.Simulate(ctx, in)
					if err != nil {
						t.Fatal(err)
					}
					active := result.Charts[0].GetActivatedSkill()
					if !active.GetActivated() || len(active.GetDetails()) != 1 || !slices.Equal(active.Details[0].GetTargetCardIndexes(), tc.want) {
						t.Fatalf("seed %d: target result %v, want %v", seed, active, tc.want)
					}
				}
			})
		}
	})
	t.Run("position triggers and equal mental passive order", func(t *testing.T) {
		for _, tc := range []struct {
			kind penum.SkillTriggerType
			want []int32
		}{
			{penum.SkillTriggerType_SkillTriggerType_Center, []int32{1}},
			{penum.SkillTriggerType_SkillTriggerType_MostLeft, []int32{4}},
			{penum.SkillTriggerType_SkillTriggerType_MostRight, []int32{5}},
			{penum.SkillTriggerType_SkillTriggerType_Beat, []int32{1, 2, 3, 4, 5}},
		} {
			in := round5EffectsFixture()
			in.Triggers["tg-round6-position"] = &pmaster.SkillTrigger{Id: "tg-round6-position", Type: tc.kind}
			effect := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 1, 20, 30, 10, "target-self")
			liveStatusSkill(in, "round6-position", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-round6-position", 10, effect)
			for lane := 1; lane <= 5; lane++ {
				round5AddSkill(in, lane, 3, "round6-position")
			}
			in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_Beat}}
			result, err := live.Simulate(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			var got []int32
			for _, activation := range result.Charts[0].GetActivatedPassiveSkills() {
				if activation.GetActivated() {
					got = append(got, activation.GetCardIndex())
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("trigger %s: activated lanes %v, want %v", tc.kind, got, tc.want)
			}
		}
	})
	t.Run("multi-recipient trigger chooses lower official lanes", func(t *testing.T) {
		in := round5EffectsFixture()
		in.Triggers["tg-someone_stamina_lower-100"] = &pmaster.SkillTrigger{Id: "tg-someone_stamina_lower-100", Type: penum.SkillTriggerType_SkillTriggerType_SomeoneStaminaLower}
		in.Targets["target-trigger-2"] = &pmaster.SkillTarget{Id: "target-trigger-2", Type: penum.SkillTargetType_SkillTargetType_Trigger}
		effect := liveStatusDefinition(in, "vocal_up", penum.SkillEfficacyType_SkillEfficacyType_VocalUp, 1, 20, 30, 10, "target-trigger-2")
		liveStatusSkill(in, "round6-trigger", penum.SkillCategoryType_SkillCategoryType_Passive, "tg-someone_stamina_lower-100", 10, effect)
		round5AddSkill(in, 5, 3, "round6-trigger")
		in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_Beat}}
		result, err := live.Simulate(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		passives := result.Charts[0].GetActivatedPassiveSkills()
		if len(passives) != 1 || !slices.Equal(passives[0].GetDetails()[0].GetTargetCardIndexes(), []int32{1, 2}) {
			t.Fatalf("trigger targets did not retain lower-numbered matching lanes: %v", passives)
		}
	})
}

func TestLiveRound6PhotoLaneScenario(t *testing.T) {
	in := liveEngineFixture(2)
	neighbors := map[int32][]int32{1: {2, 3}, 2: {1, 4}, 3: {1, 5}, 4: {2}, 5: {3}}
	for side, user := range in.Users {
		// Index, not incidental slice order, determines the official lane.
		cards := slices.Clone(user.GetUserDeck().GetCards())
		slices.Reverse(cards)
		for source := int32(1); source <= 5; source++ {
			offset := int32(side * 5)
			for _, typ := range []penum.PhotoAbilityTargetType{penum.PhotoAbilityTargetType_PhotoAbilityTargetType_Center, penum.PhotoAbilityTargetType_PhotoAbilityTargetType_Neighbor} {
				t.Run(fmt.Sprintf("side%d-source%d-%s", side, source, typ), func(t *testing.T) {
					want := []int32{offset + 1}
					if typ == penum.PhotoAbilityTargetType_PhotoAbilityTargetType_Neighbor {
						want = nil
						for _, position := range neighbors[source] {
							want = append(want, offset+position)
						}
					}
					got, err := live.PhotoTargetIndexes(cards, in.Cards, source+offset, &pmaster.PhotoAbilityTarget{Type: typ})
					slices.Sort(got)
					if err != nil || !slices.Equal(got, want) {
						t.Fatalf("photo targets %v, want %v: %v", got, want, err)
					}
				})
			}
		}
	}
}
