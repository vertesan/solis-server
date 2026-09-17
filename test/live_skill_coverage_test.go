package test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/makino/live"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/utils/config"
	"google.golang.org/protobuf/proto"
)

// This read-only audit exercises every currently configured NPC's learned
// skill definitions, including quest passives. Unsupported families are an
// explicit coverage report, not an assertion that all master skills work.
func TestLiveNPCSkillCoverage(t *testing.T) {
	ctx := context.Background()
	quests, err := opmaster.FindAllQuests(ctx)
	if err != nil {
		t.Fatal(err)
	}
	type encounter struct {
		quest   *pmaster.Quest
		roster  *papi.QuestOpponentInfo
		bonuses []*papi.LiveDeckCardSkill
	}
	var encounters []encounter
	cardIDs := map[string]bool{}
	skillIDs := map[string]bool{}
	bonusCache := map[string][]*papi.LiveDeckCardSkill{}
	for _, q := range quests {
		if q.GetClearScore() != 0 || !strings.HasPrefix(q.GetId(), "qt-main-") && !strings.HasPrefix(q.GetId(), "qt-tower-") {
			continue
		}
		roster, err := live.NPCRoster(q.GetId())
		if err != nil {
			t.Logf("unconfigured roster: %s", q.GetId())
			continue
		}
		row := encounter{quest: q, roster: roster}
		for _, c := range roster.GetCardInfos() {
			cardIDs[c.GetCardId()] = true
		}
		if groupID := q.GetLiveBonusGroupId(); groupID != "" {
			cached, ok := bonusCache[groupID]
			if !ok {
				group, err := opmaster.FindLiveBonusGroup(ctx, groupID)
				if err != nil {
					t.Fatal(err)
				}
				bonuses, err := opmaster.FindLiveBonuses(ctx, group.GetLiveBonusIds())
				if err != nil {
					t.Fatal(err)
				}
				for i, b := range bonuses {
					ability, err := opmaster.FindLiveAbility(ctx, b.GetLiveAbilityId())
					if err != nil {
						t.Fatal(err)
					}
					found := false
					for _, l := range ability.GetLevels() {
						if l.GetLevel() != b.GetLiveAbilityLevel() {
							continue
						}
						found = true
						if l.GetType() != penum.LiveAbilityType_LiveAbilityType_PassiveSkill {
							t.Logf("non-passive quest bonus: %s", b.GetId())
							continue
						}
						cached = append(cached, &papi.LiveDeckCardSkill{Index: int32(3001 + i), SkillId: l.GetSkillId(), SkillLevel: l.GetValue(), PossessionType: penum.SkillPossessionType_SkillPossessionType_Quest, LiveAbilityId: b.GetLiveAbilityId(), LiveAbilityLevel: b.GetLiveAbilityLevel()})
						skillIDs[l.GetSkillId()] = true
					}
					if !found {
						t.Fatalf("missing live bonus level: %s", b.GetId())
					}
				}
				bonusCache[groupID] = cached
			}
			row.bonuses = cached
		}
		encounters = append(encounters, row)
	}
	if len(encounters) == 0 {
		t.Fatal("no NPC rosters to audit")
	}
	cards, err := opmaster.FindLiveCards(ctx, coverageKeys(cardIDs))
	if err != nil {
		t.Fatal(err)
	}
	byCard := map[string]*pmaster.Card{}
	releaseCache := map[string][]*pmaster.CardLevelRelease{}
	for _, c := range cards {
		byCard[c.GetId()] = c
		for _, id := range []string{c.GetSkillId1(), c.GetSkillId2(), c.GetSkillId3(), c.GetSkillId4()} {
			if id != "" {
				skillIDs[id] = true
			}
		}
		if _, ok := releaseCache[c.GetCardLevelReleaseId()]; !ok {
			rows, err := opmaster.FindCardLevelReleases(ctx, c.GetCardLevelReleaseId(), 100000)
			if err != nil {
				t.Fatal(err)
			}
			releaseCache[c.GetCardLevelReleaseId()] = rows
		}
	}
	skills, err := opmaster.FindLiveSkills(ctx, coverageKeys(skillIDs))
	if err != nil {
		t.Fatal(err)
	}
	bySkill := map[string]*pmaster.Skill{}
	efficacyIDs, triggers := map[string]bool{}, map[string]bool{}
	for _, s := range skills {
		bySkill[s.GetId()] = s
		for _, l := range s.GetLevels() {
			triggers[l.GetTriggerId()] = true
			for _, d := range l.GetSkillDetails() {
				efficacyIDs[d.GetEfficacyId()] = true
				triggers[d.GetTriggerId()] = true
			}
		}
	}
	efficacies, err := opmaster.FindLiveEfficacies(ctx, coverageKeys(efficacyIDs))
	if err != nil {
		t.Fatal(err)
	}
	byEfficacy := map[string]*pmaster.SkillEfficacy{}
	targetIDs := map[string]bool{}
	for _, e := range efficacies {
		byEfficacy[e.GetId()] = e
		targetIDs[e.GetSkillTargetId()] = true
		if strings.HasPrefix(e.GetId(), "ef-score_get_by_trigger-") {
			parts := strings.Split(strings.TrimPrefix(e.GetId(), "ef-"), "-")
			if len(parts) >= 6 {
				triggers[strings.Join(parts[2:len(parts)-2], "-")] = true
			}
		}
	}
	triggerRows, err := opmaster.FindLiveTriggers(ctx, coverageKeys(triggers))
	if err != nil {
		t.Fatal(err)
	}
	byTrigger := map[string]*pmaster.SkillTrigger{}
	for _, v := range triggerRows {
		byTrigger[v.GetId()] = v
	}
	targets, err := opmaster.FindLiveTargets(ctx, coverageKeys(targetIDs))
	if err != nil {
		t.Fatal(err)
	}
	byTarget := map[string]*pmaster.SkillTarget{}
	for _, v := range targets {
		byTarget[v.GetId()] = v
	}
	settingID, err := config.GetGameSettingId()
	if err != nil {
		t.Fatal(err)
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		t.Fatal(err)
	}
	characters, err := opmaster.FindAllCharacters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byCharacter := map[string]*pmaster.Character{}
	for _, character := range characters {
		byCharacter[character.GetId()] = character
	}
	fullPassed := 0
	var totalSimulation, maximumSimulation time.Duration
	var maximumBytes int
	var maximumAllocation uint64
	unsupported := map[string]string{}
	blocked := map[string]bool{}
	cases := 0
	passed := 0
	for _, row := range encounters {
		fullRefs := map[int32][]*papi.LiveDeckCardSkill{}
		in := liveEngineFixture(2)
		in.Quest = row.quest
		in.Setting = setting
		in.Chart = []*pmaster.MusicChartPattern{{Number: 1, Type: penum.MusicChartType_MusicChartType_Beat}}
		in.Efficacies = byEfficacy
		in.Triggers = byTrigger
		in.Targets = byTarget
		in.Skills = bySkill
		// Numeric values are deliberately simple; this audit tests native definition
		// support, not the already separately tested NPC balance curve or win rates.
		for _, u := range in.Users {
			for _, c := range u.GetUserDeck().GetCards() {
				c.Skills = nil
				c.AudienceAmount = 0
			}
		}
		for _, rc := range row.roster.GetCardInfos() {
			m := byCard[rc.GetCardId()]
			if m == nil {
				t.Fatalf("missing NPC master card %q", rc.GetCardId())
			}
			c := in.Users[1].UserDeck.Cards[rc.GetPosition()-1]
			c.CardId = rc.GetCardId()
			c.Level = rc.GetLevel()
			c.Rarity = rc.GetRarity()
			data := in.Cards[c.Index]
			data.CharacterID = m.GetCharacterId()
			data.Type = m.GetType()
			in.Cards[c.Index] = data
			slots := map[int32]bool{}
			for _, r := range releaseCache[m.GetCardLevelReleaseId()] {
				if r.GetLevel() <= rc.GetLevel() {
					for _, target := range r.GetTargets() {
						if target.GetType() == penum.CardLevelReleaseType_CardLevelReleaseType_Skill {
							slots[target.GetNumber()] = true
						}
					}
				}
			}
			for i, id := range []string{m.GetSkillId1(), m.GetSkillId2(), m.GetSkillId3(), m.GetSkillId4()} {
				if id == "" || !slots[int32(i+1)] {
					continue
				}
				definition := bySkill[id]
				if definition == nil {
					t.Fatalf("missing NPC skill %s", id)
				}
				level := int32(0)
				for _, l := range definition.GetLevels() {
					if l.GetRequiredCardLevel() <= rc.GetLevel() {
						level = max(level, l.GetLevel())
					}
				}
				c.Skills = []*papi.LiveDeckCardSkill{{Index: int32(i + 1), SkillId: id, SkillLevel: level, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card}}
				fullRefs[c.Index] = append(fullRefs[c.Index], c.Skills[0])
				cases++
				_, err := live.Simulate(ctx, in)
				if err != nil {
					var u *live.UnsupportedError
					if !errors.As(err, &u) {
						t.Fatalf("unexpected NPC audit failure %s/%s: %v", row.quest.GetId(), id, err)
					}
					unsupported[u.Definition] = u.Reason
					blocked[row.quest.GetId()] = true
				} else {
					passed++
				}
				c.Skills = nil
			}
		}
		for _, bonus := range row.bonuses {
			in.Users[1].UserDeck.BonusSkills = []*papi.LiveDeckCardSkill{bonus}
			cases++
			_, err := live.Simulate(ctx, in)
			if err != nil {
				var u *live.UnsupportedError
				if !errors.As(err, &u) {
					t.Fatalf("unexpected NPC bonus audit failure %s: %v", row.quest.GetId(), err)
				}
				unsupported[u.Definition] = u.Reason
				blocked[row.quest.GetId()] = true
			} else {
				passed++
			}
			in.Users[1].UserDeck.BonusSkills = nil
		}
		duration, size, allocated, err := runNPCFullChart(ctx, in, row.roster, byCard, byCharacter, fullRefs, row.bonuses)
		if err != nil {
			t.Errorf("full authored NPC battle %s: %v", row.quest.GetId(), err)
			continue
		}
		fullPassed++
		totalSimulation += duration
		maximumSimulation = max(maximumSimulation, duration)
		maximumBytes = max(maximumBytes, size)
		maximumAllocation = max(maximumAllocation, allocated)
	}
	t.Logf("Full authored NPC battles: completed=%d/%d simulation total=%s max=%s max protobuf bytes=%d max allocated bytes/live=%d; not official-outcome validation", fullPassed, len(encounters), totalSimulation, maximumSimulation, maximumBytes, maximumAllocation)
	t.Logf("NPC coverage: rosters=%d skill cases=%d supported=%d blocked encounters=%d unsupported definitions=%d", len(encounters), cases, passed, len(blocked), len(unsupported))
	ids := make([]string, 0, len(unsupported))
	for id := range unsupported {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t.Logf("unsupported %s: %s", id, unsupported[id])
	}
	t.Logf("blocked encounters: %v", coverageKeys(blocked))
}

func runNPCFullChart(ctx context.Context, in *live.Input, roster *papi.QuestOpponentInfo, cards map[string]*pmaster.Card, characters map[string]*pmaster.Character, refs map[int32][]*papi.LiveDeckCardSkill, bonuses []*papi.LiveDeckCardSkill) (time.Duration, int, uint64, error) {
	var err error
	if in.Chart, err = opmaster.FindLiveChart(ctx, in.Quest.GetMusicChartPatternId()); err != nil {
		return 0, 0, 0, err
	}
	if in.Pressure, err = opmaster.FindLivePressure(ctx, in.Quest.GetQuestPressureId()); err != nil {
		return 0, 0, 0, err
	}
	if in.AudienceAdvantages, err = opmaster.FindLiveAudienceAdvantages(ctx, in.Quest.GetQuestAudienceAdvantageId()); err != nil {
		return 0, 0, 0, err
	}
	music, err := opmaster.FindMusic(ctx, in.Quest.GetMusicId())
	if err != nil {
		return 0, 0, 0, err
	}
	if in.ComboAdvantages, err = opmaster.FindLiveComboAdvantages(ctx, music.GetComboAdvantageId()); err != nil {
		return 0, 0, 0, err
	}
	if in.StaminaConsumption, err = opmaster.FindLiveStaminaConsumption(ctx); err != nil {
		return 0, 0, 0, err
	}
	balance, err := live.NPCBalance(in.Quest, in.Setting)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, rc := range roster.GetCardInfos() {
		master := cards[rc.GetCardId()]
		base, err := usercard.CalcUserCardBaseAttr(ctx, master, rc.GetLevel(), rc.GetRarity())
		if err != nil {
			return 0, 0, 0, err
		}
		npc := in.Users[1].UserDeck.Cards[rc.GetPosition()-1]
		npc.Vocal, npc.Dance, npc.Visual = base.Vocal+balance.Primary, base.Dance+balance.Primary, base.Visual+balance.Primary
		npc.Stamina, npc.Mental, npc.Technique = base.Stamina+balance.Stamina, balance.Mental, balance.Technique
		npc.StartStamina = npc.Stamina
		npc.Skills = refs[npc.Index]
		data := live.CardData{CharacterID: master.GetCharacterId(), GroupID: characters[master.GetCharacterId()].GetCharacterGroupId(), Type: master.GetType(), LightFans: int64(rc.GetLightFanAmount()), MiddleFans: int64(rc.GetMiddleFanAmount()), HeavyFans: int64(rc.GetHeavyFanAmount()), Attribute: penum.AttributeType_AttributeType_Vocal}
		highest := master.GetVocalRatioPermil()
		if master.GetDanceRatioPermil() > highest {
			data.Attribute, highest = penum.AttributeType_AttributeType_Dance, master.GetDanceRatioPermil()
		}
		if master.GetVisualRatioPermil() > highest {
			data.Attribute = penum.AttributeType_AttributeType_Visual
		}
		in.Cards[npc.Index] = data
		// Synthetic caller uses the same five kits with slightly stronger primary
		// stats. This stresses both sides without reading or mutating any player.
		player := proto.Clone(npc).(*papi.LiveDeckCard)
		player.Index -= 5
		player.Vocal, player.Dance, player.Visual = player.Vocal*11/10, player.Dance*11/10, player.Visual*11/10
		in.Users[0].UserDeck.Cards[rc.GetPosition()-1] = player
		in.Cards[player.Index] = data
	}
	var audienceData []live.CardData
	for _, user := range in.Users {
		user.UserDeck.BonusSkills = bonuses
		for _, card := range user.UserDeck.Cards {
			audienceData = append(audienceData, in.Cards[card.Index])
		}
	}
	audience, err := live.AllocateAudience(audienceData, in.Quest.GetMaxCapacity(), in.Setting)
	if err != nil {
		return 0, 0, 0, err
	}
	index := 0
	for _, user := range in.Users {
		for _, card := range user.UserDeck.Cards {
			card.AudienceAmount = audience[index]
			index++
		}
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	result, err := live.Simulate(ctx, in)
	elapsed := time.Since(started)
	runtime.ReadMemStats(&after)
	if err != nil {
		return elapsed, 0, after.TotalAlloc - before.TotalAlloc, err
	}
	if err := checkLiveScoreConservation(result); err != nil {
		return elapsed, 0, after.TotalAlloc - before.TotalAlloc, err
	}
	wantCharts := 0
	for _, chart := range in.Chart {
		if chart.GetType() != penum.MusicChartType_MusicChartType_Unknown {
			wantCharts++
		}
	}
	if len(result.GetCharts()) != wantCharts {
		return elapsed, 0, after.TotalAlloc - before.TotalAlloc, fmt.Errorf("chart count %d != %d", len(result.GetCharts()), wantCharts)
	}
	return elapsed, proto.Size(result), after.TotalAlloc - before.TotalAlloc, nil
}

func checkLiveScoreConservation(result *papi.LiveResult) error {
	var trace [2]int64
	add := func(index int32, score int64) error {
		if score < 0 {
			return fmt.Errorf("negative score")
		}
		if score == 0 {
			return nil
		}
		side := 0
		if index >= 6 && index <= 10 || index == 1000 {
			side = 1
		} else if !(index >= 1 && index <= 5 || index == 999) {
			return fmt.Errorf("invalid scoring actor %d", index)
		}
		trace[side] += score
		return nil
	}
	for _, skill := range result.GetActivatedPassiveSkills() {
		if err := add(skill.GetCardIndex(), skill.GetScore()); err != nil {
			return err
		}
	}
	for _, chart := range result.GetCharts() {
		for _, beat := range chart.GetBeats() {
			if err := add(beat.GetCardIndex(), beat.GetScore()); err != nil {
				return err
			}
		}
		if skill := chart.GetActivatedSkill(); skill != nil {
			if err := add(skill.GetCardIndex(), skill.GetScore()); err != nil {
				return err
			}
		}
		for _, skill := range chart.GetActivatedPassiveSkills() {
			if err := add(skill.GetCardIndex(), skill.GetScore()); err != nil {
				return err
			}
		}
	}
	for i, user := range result.GetUserInfos() {
		var cards int64
		for _, card := range user.GetUserResult().GetCardResults() {
			cards += card.GetTotalScore()
		}
		if cards != user.GetUserResult().GetScore() || trace[i] != cards {
			return fmt.Errorf("side%d score mismatch: trace=%d cards=%d result=%d", i, trace[i], cards, user.GetUserResult().GetScore())
		}
	}
	last := result.GetCharts()[len(result.GetCharts())-1]
	for _, status := range last.GetUserStatuses() {
		if status.GetTotalScore() != result.GetUserInfos()[status.GetUserIndex()-1].GetUserResult().GetScore() {
			return fmt.Errorf("final snapshot score mismatch")
		}
	}
	return nil
}

func coverageKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for id := range set {
		if id != "" {
			keys = append(keys, id)
		}
	}
	sort.Strings(keys)
	return keys
}
