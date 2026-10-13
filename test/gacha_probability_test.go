package test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/proto"
)

// Browsing probabilities must work for every available gacha, including pools
// whose featured cards exhaust a rarity's budget and leave other cards at zero.
func TestGachaProbabilityCatalogScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	client := papi.NewGachaClient(testConfig.GrpcConn)
	list := profileCall(t, client.List, &papi.Empty{})
	cards, err := opmaster.FindAllCards(context.Background())
	if err != nil || len(cards) == 0 {
		t.Fatalf("normal gacha requires the current master card catalog: %v", err)
	}
	rarityBudgets := testNormalGachaRarityBudgets(t)
	cardIDs := make(map[string]int32, len(cards))
	rarityCounts := make(map[int32]int64, len(rarityBudgets))
	for _, card := range cards {
		if rarityBudgets[card.GetInitialRarity()] <= 0 {
			continue
		}
		cardIDs[card.GetId()] = card.GetInitialRarity()
		rarityCounts[card.GetInitialRarity()]++
	}
	checked := 0
	var normal *papi.GachaInfo
	var normalProbability *papi.GachaProbabilityResponse
	for _, info := range list.GetGachas() {
		if !info.GetUnlocked() {
			continue
		}
		master, err := opmaster.FindGacha(context.Background(), info.GetId())
		if err != nil {
			t.Fatal(err)
		}
		response := profileCall(t, client.Probability, &papi.GachaProbabilityRequest{GachaId: info.GetId()})
		checked++
		if len(response.GetProbabilityInfos()) == 0 {
			t.Fatalf("Gacha.Probability(%q) returned no drawable rewards", info.GetId())
		}
		var total int64
		for _, entry := range response.GetProbabilityInfos() {
			if entry.GetProbability() <= 0 {
				t.Fatalf("Gacha.Probability(%q) advertised an undrawable reward: %v", info.GetId(), entry)
			}
			total += int64(entry.GetProbability())
		}
		isNormal := master.GetGachaType() == penum.GachaType_GachaType_Normal && strings.HasPrefix(info.GetId(), "gacha-normal-")
		// Premium catalogs and normal pools intentionally truncate
		// their displayed odds; their exact contracts are checked separately.
		if !isNormal && master.GetGachaType() != penum.GachaType_GachaType_Premium && total != 100000 {
			t.Fatalf("Gacha.Probability(%q) probability total = %d, want 100000", info.GetId(), total)
		}
		for _, rarity := range response.GetRarityProbabilityInfos() {
			var rarityTotal int32
			for _, entry := range rarity.GetProbabilityInfos() {
				rarityTotal += entry.GetProbability()
				found := false
				for _, overall := range response.GetProbabilityInfos() {
					if proto.Equal(entry, overall) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("Gacha.Probability(%q) rarity table has a reward missing from its overall table", info.GetId())
				}
			}
			if isNormal {
				budget := rarityBudgets[rarity.GetRarity()]
				count := rarityCounts[rarity.GetRarity()]
				if budget <= 0 || count == 0 || int64(len(rarity.GetProbabilityInfos())) != count {
					t.Fatalf("normal gacha %q rarity-%d does not include its complete master pool", info.GetId(), rarity.GetRarity())
				}
				// The header reports the actual tier odds; individual card
				// percentages can sum slightly lower because display is truncated.
				if int64(rarity.GetProbability()) != budget || int64(rarity.GetIndividualProbability()) != budget/count {
					t.Fatalf("normal gacha %q rarity-%d header = %d, individual = %d; want %d and %d", info.GetId(), rarity.GetRarity(), rarity.GetProbability(), rarity.GetIndividualProbability(), budget, budget/count)
				}
			} else if rarityTotal != rarity.GetProbability() {
				t.Fatalf("Gacha.Probability(%q) rarity-%d total = %d, entries total = %d", info.GetId(), rarity.GetRarity(), rarity.GetProbability(), rarityTotal)
			}
		}
		if isNormal {
			// Normal pools include all master cards in configured rarity tiers,
			// including limited cards. Pickup IDs must not change their odds.
			if len(response.GetProbabilityInfos()) != len(cardIDs) {
				t.Fatalf("normal gacha %q contains %d cards, want all %d eligible master cards", info.GetId(), len(response.GetProbabilityInfos()), len(cardIDs))
			}
			if len(response.GetRarityProbabilityInfos()) != len(rarityCounts) {
				t.Fatalf("normal gacha %q omitted a configured rarity tier", info.GetId())
			}
			seen := make(map[string]bool, len(cardIDs))
			for _, entry := range response.GetProbabilityInfos() {
				rarity, exists := cardIDs[entry.GetResourceId()]
				if !exists || seen[entry.GetResourceId()] || entry.GetResourceType() != penum.ResourceType_ResourceType_Card || entry.GetAmount() != 1 {
					t.Fatalf("normal gacha %q has an invalid or duplicate card reward: %v", info.GetId(), entry)
				}
				seen[entry.GetResourceId()] = true
				individual := int32(rarityBudgets[rarity] / rarityCounts[rarity])
				if entry.GetProbability() != individual || entry.GetIsRatioUp() {
					t.Fatalf("normal gacha %q does not advertise equal rarity-%d card odds (%d): %v", info.GetId(), rarity, individual, entry)
				}
			}
			normal, normalProbability = info, response
		}
	}
	if checked == 0 || normal == nil {
		t.Fatal("current gacha catalog contains no unlocked normal card gacha")
	}
	_, _, button, consumption := chooseGachaDrawTarget(t, []*papi.GachaInfo{normal})
	if button.GetDrawCount() != 1 {
		t.Fatal("normal gacha regression requires a single-draw button, not a guaranteed-rarity batch")
	}
	costType, costID, costAmount := testGachaConsumption(consumption)
	resourceBefore := testGachaResourceAmount(t, costType, costID)
	buttonBefore := findTestGachaButton(normal, button.GetId())
	owned, err := opuser.FindAllUserCards(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	cardsBefore := make(map[string]*ptransaction.UserCard, len(owned))
	for _, card := range owned {
		cardsBefore[card.GetCardId()] = card
	}
	missionsBefore := testGachaMissionSnapshot(t)
	giftsBefore := testPendingGachaGiftAmount(t, penum.ResourceType_ResourceType_Item, "item-card_limit_break")
	notifiedBefore := testGachaGiftNotified(t)
	draw := profileCall(t, client.Draw, &papi.GachaDrawRequest{GachaButtonId: button.GetId(), ExecCount: 1})
	if len(draw.GetRewards()) != 1 {
		t.Fatalf("single draw returned %d rewards", len(draw.GetRewards()))
	}
	for _, reward := range draw.GetRewards() {
		found := false
		for _, entry := range normalProbability.GetProbabilityInfos() {
			if reward.GetResourceType() == entry.GetResourceType() && reward.GetResourceId() == entry.GetResourceId() && reward.GetAmount() == entry.GetAmount() {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Gacha.Draw returned a reward absent from its probability table: %v", reward)
		}
	}
	if got := testGachaResourceAmount(t, costType, costID); got != resourceBefore-costAmount {
		t.Fatalf("normal gacha did not persist draw cost: got %d, want %d", got, resourceBefore-costAmount)
	}
	stored, err := opuser.FindUserGachaInfo(context.Background(), testConfig.TestUserSuid, normal.GetId())
	if err != nil {
		t.Fatal(err)
	}
	storedButton := findTestGachaButton(stored, button.GetId())
	if !proto.Equal(storedButton, draw.GetButton()) || draw.GetButton().GetTotalCount() != buttonBefore.GetTotalCount()+1 {
		t.Fatalf("normal gacha did not persist its draw count: stored=%v response=%v", storedButton, draw.GetButton())
	}
	assertGachaDuplicateCardProgress(t, cardsBefore, missionsBefore, giftsBefore, notifiedBefore, draw)
	// Cards are uniform within rarity tiers, and ten-pulls promise at least one
	// ★4+ card. Assert the guarantee without depending on any random outcome.
	checkedBatch := false
	for _, state := range normal.GetButtons() {
		batch, err := opmaster.FindGachaButton(context.Background(), state.GetId())
		if err != nil {
			t.Fatal(err)
		}
		if batch.GetDrawCount() != 10 {
			continue
		}
		batchDraw := profileCall(t, client.Draw, &papi.GachaDrawRequest{GachaButtonId: batch.GetId(), ExecCount: 1})
		if len(batchDraw.GetRewards()) != 10 {
			t.Fatalf("normal ten-pull returned %d rewards", len(batchDraw.GetRewards()))
		}
		for _, reward := range batchDraw.GetRewards() {
			if _, exists := cardIDs[reward.GetResourceId()]; !exists || reward.GetResourceType() != penum.ResourceType_ResourceType_Card || reward.GetAmount() != 1 {
				t.Fatalf("normal ten-pull returned a reward outside the card catalog: %v", reward)
			}
		}
		assertGachaCardBatchGuarantee(t, batchDraw.GetRewards(), batch.GetDrawCount())
		checkedBatch = true
		break
	}
	if !checkedBatch {
		t.Fatal("normal gacha has no ten-pull button to check its guaranteed rarity")
	}
	t.Logf("checked probability tables for %d unlocked gachas", checked)
}

func testNormalGachaRarityBudgets(t *testing.T) map[int32]int64 {
	t.Helper()
	data, err := os.ReadFile("../internal/server/rpc/servers/internal/viber/catalogdata/gacha.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var rules struct {
		Probabilities map[int32]int64 `yaml:"standardCardRarityProbabilities"`
	}
	if err := yaml.Unmarshal(data, &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules.Probabilities) == 0 {
		t.Fatal("normal gacha has no configured rarity budgets")
	}
	return rules.Probabilities
}
