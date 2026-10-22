package test

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
)

// Changes to the administrative catalog become visible on the next request,
// including reopening old events. Both singleton documents are restored even
// when an assertion fails; all player mutations use the configured test user.
func TestMakinoGachaRotationScenario(t *testing.T) {
	backupMakinoCatalogCollections(t, sname.GachaCatalog, sname.GachaRules)
	ctx := context.Background()
	originalRules, err := opsolis.FindGachaRules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	setupTestUser(t)
	client := papi.NewGachaClient(testConfig.GrpcConn)
	setRotation := func(rotation *smodel.GachaCatalog) {
		t.Helper()
		if err := opsolis.UpsertGachaCatalog(ctx, rotation); err != nil {
			t.Fatal(err)
		}
		stored, err := opsolis.FindGachaCatalog(ctx)
		if err != nil || stored == nil || stored.UseMasterCatalog != rotation.UseMasterCatalog ||
			stored.IgnoreOfficialDates != rotation.IgnoreOfficialDates || !slices.Equal(stored.GachaIds, rotation.GachaIds) {
			t.Fatalf("rotation BSON roundtrip: got=%v want=%v err=%v", stored, rotation, err)
		}
	}
	setRotation(&smodel.GachaCatalog{UseMasterCatalog: true, IgnoreOfficialDates: true})
	baseline := profileCall(t, client.List, &papi.Empty{})
	baselineIDs := make(map[string]bool)
	normalID := ""
	for _, entry := range baseline.GetGachas() {
		baselineIDs[entry.GetId()] = true
		if strings.HasPrefix(entry.GetId(), "gacha-normal-") {
			normalID = entry.GetId()
		}
	}
	if normalID == "" {
		t.Fatal("master rotation has no visible normal gacha")
	}
	oldGacha, button := testExpiredDiamondGacha(t)
	rotation := &smodel.GachaCatalog{GachaIds: []string{oldGacha.GetId()}}
	setRotation(rotation)
	if got := profileCall(t, client.List, &papi.Empty{}); len(got.GetGachas()) != 0 {
		t.Fatalf("expired gacha reopened without date override: %v", got.GetGachas())
	}
	profileReject(t, codes.PermissionDenied, client.Probability, &papi.GachaProbabilityRequest{GachaId: oldGacha.GetId()})
	rotation.IgnoreOfficialDates = true
	setRotation(rotation)
	if got := profileCall(t, client.List, &papi.Empty{}); len(got.GetGachas()) != 1 || got.GetGachas()[0].GetId() != oldGacha.GetId() {
		t.Fatalf("explicit roster was not reopened immediately: %v", got.GetGachas())
	}
	if got := profileCall(t, client.Probability, &papi.GachaProbabilityRequest{GachaId: oldGacha.GetId()}); len(got.GetProbabilityInfos()) == 0 {
		t.Fatal("reopened gacha has no probability table")
	}
	setDiamondTestBalance(t, 0, 0)
	profileReject(t, codes.ResourceExhausted, client.Draw, &papi.GachaDrawRequest{GachaButtonId: button.GetId(), ExecCount: 1})
	setDiamondTestBalance(t, 100000, 100000)
	draw := profileCall(t, client.Draw, &papi.GachaDrawRequest{GachaButtonId: button.GetId(), ExecCount: 1})
	if len(draw.GetRewards()) == 0 || len(draw.GetConsumptionResults()) == 0 {
		t.Fatal("reopened draw did not grant rewards and consume resources")
	}
	if oldGacha.GetExchangeResourceType() == penum.ResourceType_ResourceType_Item && oldGacha.GetExchangeResourceId() != "" {
		items, err := opuser.FindUserItems(ctx, testConfig.TestUserSuid, oldGacha.GetExchangeResourceId())
		if err != nil || len(items) == 0 {
			t.Fatalf("missing reopened gacha exchange resource: %v", err)
		}
		for _, item := range items {
			if item.GetExpiredTime() != 0 {
				t.Fatalf("reopened gacha granted an officially expired item: %v", item)
			}
		}
	}

	// Bypassing official dates must not bypass inventory prerequisites.
	const ticketGachaID = "gacha-free-ticket-1s-26-0910"
	const ticketItemID = "item-gacha-free-ticket-1s-26-0910"
	rotation.GachaIds = append(rotation.GachaIds, ticketGachaID)
	setRotation(rotation)
	if got := profileCall(t, client.List, &papi.Empty{}); len(got.GetGachas()) != 1 {
		t.Fatalf("ticket-less gacha became visible: %v", got.GetGachas())
	}
	profileReject(t, codes.PermissionDenied, client.Probability, &papi.GachaProbabilityRequest{GachaId: ticketGachaID})
	if err := opuser.UpsertUserItem(ctx, testConfig.TestUserSuid, &ptransaction.UserItem{ItemId: ticketItemID, Amount: 1}); err != nil {
		t.Fatal(err)
	}
	if got := profileCall(t, client.List, &papi.Empty{}); len(got.GetGachas()) != 2 {
		t.Fatalf("owned ticket did not unlock explicit gacha: %v", got.GetGachas())
	}

	// Add mode preserves the automatic roster, without reopening every old
	// master entry merely because the explicit extra ignores official dates.
	rotation.UseMasterCatalog = true
	rotation.GachaIds = []string{oldGacha.GetId()}
	setRotation(rotation)
	added := profileCall(t, client.List, &papi.Empty{})
	if len(added.GetGachas()) != len(baseline.GetGachas())+1 {
		t.Fatalf("add mode changed unrelated master entries: before=%d after=%d", len(baseline.GetGachas()), len(added.GetGachas()))
	}
	for _, entry := range added.GetGachas() {
		if entry.GetId() != oldGacha.GetId() && !baselineIDs[entry.GetId()] {
			t.Fatalf("unselected master gacha reopened: %s", entry.GetId())
		}
	}
	modifiedRules := *originalRules
	modifiedRules.StandardCardRarityProbabilities = map[int32]int64{3: 80000, 4: 10000, 5: 10000}
	if err := opsolis.UpsertGachaRules(ctx, &modifiedRules); err != nil {
		t.Fatal(err)
	}
	storedRules, err := opsolis.FindGachaRules(ctx)
	if err != nil || !reflect.DeepEqual(storedRules, &modifiedRules) {
		t.Fatalf("gacha rules BSON roundtrip, including integer rarity keys: err=%v", err)
	}
	probability := profileCall(t, client.Probability, &papi.GachaProbabilityRequest{GachaId: normalID})
	for _, tier := range probability.GetRarityProbabilityInfos() {
		if int64(tier.GetProbability()) != modifiedRules.StandardCardRarityProbabilities[tier.GetRarity()] {
			t.Fatalf("probability retained old rules after DB update: %v", tier)
		}
	}

	setRotation(&smodel.GachaCatalog{})
	if got := profileCall(t, client.List, &papi.Empty{}); len(got.GetGachas()) != 0 {
		t.Fatal("cleared roster fell back to master data")
	}
	profileReject(t, codes.PermissionDenied, client.Draw, &papi.GachaDrawRequest{GachaButtonId: button.GetId(), ExecCount: 1})
	home := profileCall(t, papi.NewHomeClient(testConfig.GrpcConn).Enter, &papi.HomeEnterRequest{})
	if len(home.GetGachaInfos()) != 0 {
		t.Fatal("Home.Enter returned stale gachas after clearing the roster")
	}
	user := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	if len(user.GetGachaButtons()) != 0 {
		t.Fatal("User.Get returned stale gacha buttons after clearing the roster")
	}
}

func testExpiredDiamondGacha(t *testing.T) (*pmaster.Gacha, *pmaster.GachaButton) {
	t.Helper()
	ctx := context.Background()
	masters, err := opmaster.FindAllGachas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range masters {
		if !strings.Contains(candidate.GetId(), "birthday") || candidate.GetUnlockConditionId() != "" {
			continue
		}
		condition, err := opmaster.FindCondition(ctx, candidate.GetViewConditionId())
		if err != nil || len(condition.GetSettings()) != 1 {
			continue
		}
		term := condition.GetSettings()[0].GetTerm()
		if term.GetNowBefore() <= 0 || term.GetNowBefore() >= time.Now().UnixMilli() {
			continue
		}
		for _, id := range candidate.GetButtonIds() {
			button, err := opmaster.FindGachaButton(ctx, id)
			if err != nil || button.GetDrawCount() != 1 || button.GetConsumptionId() == "" || button.GetDiscountConsumptionId() != "" {
				continue
			}
			consumption, err := opmaster.FindConsumption(ctx, button.GetConsumptionId())
			if err != nil || len(consumption.GetResources()) != 1 {
				continue
			}
			if resource := consumption.GetResources()[0]; resource.GetResourceType() == penum.ResourceType_ResourceType_Stone && resource.GetAmount() > 0 {
				return candidate, button
			}
		}
	}
	t.Fatal("no expired birthday gacha with an ordinary diamond button exists")
	return nil, nil
}
