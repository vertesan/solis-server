package test

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/live"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

func beatCoefficientChart(beats int) []*pmaster.MusicChartPattern {
	rows := make([]*pmaster.MusicChartPattern, 0, beats+3)
	for i := 0; i < beats; i++ {
		rows = append(rows, &pmaster.MusicChartPattern{Number: int32(i + 1), Type: penum.MusicChartType_MusicChartType_Beat})
	}
	// These records must not contribute to the normal-beat count.
	return append(rows,
		&pmaster.MusicChartPattern{Number: int32(beats + 1), Type: penum.MusicChartType_MusicChartType_Unknown},
		&pmaster.MusicChartPattern{Number: int32(beats + 2), Type: penum.MusicChartType_MusicChartType_ActiveSkill, Position: 1},
		&pmaster.MusicChartPattern{Number: int32(beats + 3), Type: penum.MusicChartType_MusicChartType_SpecialSkill, Position: 1})
}

func TestLiveBeatCoefficientResolutionScenario(t *testing.T) {
	setupTestUser(t)
	for _, tc := range []struct {
		name, music       string
		reference, active int
		want              int32
	}{
		{"standard chart instead of active variant", "music-tri-001", 140, 145, 57},
		{"different song", "music-hsm-005", 154, 161, 52},
		{"round down", "music-moon-004", 149, 159, 53},
		{"round exact half upward", "fixture-half-rounding", 127, 140, 63},
		{"observed exception overrides inferred count", "music-hsm-001", 133, 137, 57},
		{"missing reference uses active chart", "fixture-no-reference", -1, 145, 55},
		{"empty reference uses active chart", "fixture-empty-reference", 0, 145, 55},
		{"skill-only chart needs no coefficient", "fixture-skill-only", 0, 0, 0},
		{"one beat upper coefficient", "fixture-short-chart", -1, 1, 4000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reference []*pmaster.MusicChartPattern
			if tc.reference >= 0 {
				reference = beatCoefficientChart(tc.reference)
			}
			got, err := live.BeatScoreCoefficientPermil(tc.music, reference, beatCoefficientChart(tc.active))
			if err != nil || got != tc.want {
				t.Fatalf("coefficient=%d want%d: %v", got, tc.want, err)
			}
		})
	}
}

func TestLiveOrdinaryCapturedBeatCoefficientScenario(t *testing.T) {
	setupTestUser(t)
	// Ordinary main live, not Exercise: newbie_1/021-QuestStartResponse,
	// chart8/lane2. No photo, buff, critical, combo or character score bonus.
	const capturedScore int64 = 95
	const vocal, dance, visual int64 = 1215, 1778, 1463
	const audienceFactor = 1.001
	coefficient, err := live.BeatScoreCoefficientPermil("music-tri-001", beatCoefficientChart(140), beatCoefficientChart(145))
	if err != nil || coefficient != 57 {
		t.Fatalf("captured song coefficient=%d: %v", coefficient, err)
	}
	weighted := vocal*150/1000 + dance*600/1000 + visual*250/1000
	base := float64(weighted) * float64(coefficient) / 1000 * audienceFactor
	if weighted != 1613 || int64(math.Floor(base*1.033)) != capturedScore {
		t.Fatal("captured score no longer fits weighted per-term floors and K57")
	}
	oldWeighted := (float64(vocal)*150 + float64(dance)*600 + float64(visual)*250) / 1000
	if maximum := int64(math.Floor(oldWeighted * 8 / 145 * audienceFactor * 1.05)); maximum != 93 || maximum >= capturedScore {
		t.Fatal("capture fixture no longer distinguishes the old 8/N formula")
	}
	in := liveEngineFixture(1)
	in.Quest.MusicId = "music-tri-001"
	in.Quest.MaxCapacity = 50
	in.Quest.BeatVocalWeightPermil, in.Quest.BeatDanceWeightPermil, in.Quest.BeatVisualWeightPermil = 150, 600, 250
	in.BeatScoreCoefficientPermil = coefficient
	in.AudienceAdvantages = []*pmaster.QuestAudienceAdvantage{{AudienceAmount: 10, AdvantagePermil: 1001}}
	in.Chart = beatCoefficientChart(145)[:145]
	for _, card := range in.Users[0].UserDeck.Cards {
		card.Vocal, card.Dance, card.Visual = vocal, dance, visual
		card.AudienceAmount = 10
		card.Technique = 0
		card.Skills = nil
	}
	for _, chart := range round5Simulate(t, in).GetCharts() {
		for _, beat := range chart.GetBeats() {
			if beat.GetIsCritical() || beat.GetScore() < 87 || beat.GetScore() > 96 {
				t.Fatalf("ordinary beat does not use the inferred song coefficient: %v", beat)
			}
			assertLiveScoreFormula(t, beat.GetScore(), base, 0)
		}
	}
}

func TestQuestReferenceBeatCoefficientScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	q, err := opmaster.FindQuest(ctx, "qt-main-1-003-01")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := opmaster.FindLiveChart(ctx, "chart-"+strings.TrimPrefix(q.GetMusicId(), "music-")+"-001")
	if err != nil {
		t.Fatal(err)
	}
	active, err := opmaster.FindLiveChart(ctx, q.GetMusicChartPatternId())
	if err != nil {
		t.Fatal(err)
	}
	coefficient, err := live.BeatScoreCoefficientPermil(q.GetMusicId(), reference, active)
	if err != nil {
		t.Fatal(err)
	}
	variantCoefficient, err := live.BeatScoreCoefficientPermil(q.GetMusicId(), nil, active)
	if err != nil || coefficient == variantCoefficient || q.GetQuestCharacterAdvantageId() != "" || q.GetLiveBonusGroupId() != "" {
		t.Fatalf("quest fixture must distinguish reference and active charts without extra quest bonuses: %v", err)
	}
	areas, _ := questCatalogFixture(t)
	area, err := opmaster.FindArea(ctx, q.GetAreaId())
	if err != nil {
		t.Fatal(err)
	}
	for _, previous := range questAreasOfType(areas, penum.AreaType_AreaType_Main) {
		if previous.GetOrder() < area.GetOrder() {
			questSeedFullArea(t, previous)
		}
	}
	maximum, err := opmaster.FindMaxManagerLevel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	questSetManagerLevel(t, maximum.GetLevel())
	number := questStartDeck(t, 1_000_000)
	// The fixture has no equipment. Disable only this temporary user's global
	// abilities so the prepared response stats fully describe beat-score input.
	cards, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, card := range cards {
		card.LiveAbilityLevel = 0
		if err := opuser.UpsertUserCard(ctx, testConfig.TestUserSuid, card); err != nil {
			t.Fatal(err)
		}
	}
	music, err := opmaster.FindMusic(ctx, q.GetMusicId())
	if err != nil {
		t.Fatal(err)
	}
	combos, err := opmaster.FindLiveComboAdvantages(ctx, music.GetComboAdvantageId())
	if err != nil {
		t.Fatal(err)
	}
	audiences, err := opmaster.FindLiveAudienceAdvantages(ctx, q.GetQuestAudienceAdvantageId())
	if err != nil {
		t.Fatal(err)
	}
	response := profileCall(t, papi.NewQuestClient(testConfig.GrpcConn).Start, &papi.QuestStartRequest{QuestId: q.GetId(), DeckNumber: number, PlayCount: 1, IsSkip: true})
	result := response.GetResult()
	prepared := map[int32]*papi.LiveDeckCard{}
	for _, card := range result.GetUserInfos()[0].GetUserDeck().GetCards() {
		prepared[card.GetIndex()] = card
	}
	combo := int32(0)
	checked := 0
	for _, chart := range result.GetCharts() {
		// Restrict the check to the clean opening, before native skills can
		// alter any score/attribute/audience effects or combo timing.
		if chart.GetActivatedSkill().GetActivated() {
			break
		}
		activatedPassive := false
		for _, skill := range chart.GetActivatedPassiveSkills() {
			activatedPassive = activatedPassive || skill.GetActivated()
		}
		if activatedPassive {
			break
		}
		comboFactor := 1.0
		for _, row := range combos {
			if row.GetComboCount() <= combo {
				comboFactor = max(comboFactor, float64(row.GetAdvantagePermil())/1000)
			}
		}
		for _, beat := range chart.GetBeats() {
			if beat.GetIsCritical() {
				continue
			}
			card := prepared[beat.GetCardIndex()]
			audienceFactor, threshold := 1.0, int32(-1)
			for _, row := range audiences {
				if row.GetAudienceAmount() <= card.GetAudienceAmount() && row.GetAudienceAmount() > threshold {
					audienceFactor, threshold = float64(row.GetAdvantagePermil())/1000, row.GetAudienceAmount()
				}
			}
			weighted := card.GetVocal()*int64(q.GetBeatVocalWeightPermil())/1000 + card.GetDance()*int64(q.GetBeatDanceWeightPermil())/1000 + card.GetVisual()*int64(q.GetBeatVisualWeightPermil())/1000
			base := float64(weighted) * float64(coefficient) / 1000 * audienceFactor * comboFactor
			assertLiveScoreFormula(t, beat.GetScore(), base, 0)
			checked++
		}
		combo = chart.GetUserStatuses()[0].GetCurrentComboCount()
	}
	// Do not require a random roll outside the overlapping variant range:
	// every returned score must fit the reference formula, regardless of luck.
	if checked < 5 {
		t.Fatalf("quest adapter provided too few clean reference-chart beats: %d", checked)
	}
}
