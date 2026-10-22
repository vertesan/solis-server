package test

import (
	"bytes"
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/makino/catalog"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"go.yaml.in/yaml/v3"
)

func TestMakinoGachaValidationScenario(t *testing.T) {
	backupMakinoCatalogCollections(t, sname.GachaCatalog, sname.GachaRules)
	setupTestUser(t)
	ctx := context.Background()
	export := func(kind string) []byte {
		t.Helper()
		data, err := catalog.ExportGacha(ctx, kind)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	encode := func(value any) [][]byte {
		t.Helper()
		data, err := yaml.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return [][]byte{data}
	}
	before := export("gacha-rules")
	if err := catalog.ApplyGacha(ctx, "gacha-rules", "overwrite", [][]byte{before}); err != nil {
		t.Fatalf("existing rule export must be reimportable: %v", err)
	}
	if !bytes.Equal(before, export("gacha-rules")) {
		t.Fatal("export/import changed gacha rules")
	}
	var originalRules smodel.GachaRules
	if err := yaml.Unmarshal(before, &originalRules); err != nil {
		t.Fatal(err)
	}
	if len(originalRules.ItemGachas) == 0 || len(originalRules.PremiumItems) == 0 || len(originalRules.ContinuousGachas) == 0 || len(originalRules.GuaranteedRarityGachas) == 0 {
		t.Fatal("configured rules lack pools needed for validation regression")
	}
	for _, testcase := range []struct {
		name string
		edit func(*smodel.GachaRules)
	}{
		{"premium item", func(rules *smodel.GachaRules) { rules.PremiumItems[0].ItemID = "missing-makino-gacha-item" }},
		{"pool resource", func(rules *smodel.GachaRules) {
			rules.ItemGachas[0].Rewards[0].ResourceID = "missing-makino-gacha-resource"
		}},
		{"unsupported pool type", func(rules *smodel.GachaRules) {
			rules.ItemGachas[0].Rewards[0].ResourceType = int32(penum.ResourceType_ResourceType_Unknown) + 1000000
		}},
		{"continuous reward", func(rules *smodel.GachaRules) { rules.ContinuousGachas[0].RewardID = "missing-makino-gacha-reward" }},
		{"guaranteed gacha", func(rules *smodel.GachaRules) { rules.GuaranteedRarityGachas[0].ID = "missing-makino-gacha" }},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			var rules smodel.GachaRules
			if err := yaml.Unmarshal(before, &rules); err != nil {
				t.Fatal(err)
			}
			testcase.edit(&rules)
			if err := catalog.ApplyGacha(ctx, "gacha-rules", "overwrite", encode(&rules)); err == nil {
				t.Fatal("invalid gacha rules were accepted")
			}
			if !bytes.Equal(before, export("gacha-rules")) {
				t.Fatal("rejected import modified the saved rules")
			}
		})
	}

	// An unselected historical pool can be removed, but must be configured
	// again before its gacha can be added to the active rotation.
	automatic := &smodel.GachaCatalog{UseMasterCatalog: true, IgnoreOfficialDates: true}
	if err := catalog.ApplyGacha(ctx, "gacha", "overwrite", encode(automatic)); err != nil {
		t.Fatal(err)
	}
	rotationBefore := export("gacha")
	poolID := originalRules.ItemGachas[0].ID
	withoutPool := originalRules
	withoutPool.ItemGachas = append([]smodel.GachaCatalogPool(nil), originalRules.ItemGachas[1:]...)
	if err := catalog.ApplyGacha(ctx, "gacha-rules", "overwrite", encode(&withoutPool)); err != nil {
		t.Fatalf("remove an unselected pool: %v", err)
	}
	missingPoolRules := export("gacha-rules")
	explicit := &smodel.GachaCatalog{GachaIds: []string{poolID}, IgnoreOfficialDates: true}
	if err := catalog.ApplyGacha(ctx, "gacha", "overwrite", encode(explicit)); err == nil {
		t.Fatal("enabled an item gacha with no configured reward pool")
	}
	if !bytes.Equal(rotationBefore, export("gacha")) || !bytes.Equal(missingPoolRules, export("gacha-rules")) {
		t.Fatal("rejected roster import modified a catalog")
	}
	if err := catalog.ApplyGacha(ctx, "gacha-rules", "overwrite", [][]byte{before}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.ApplyGacha(ctx, "gacha", "overwrite", encode(explicit)); err != nil {
		t.Fatalf("enable configured item pool: %v", err)
	}
	rotationBefore = export("gacha")
	if err := catalog.ApplyGacha(ctx, "gacha-rules", "overwrite", encode(&withoutPool)); err == nil {
		t.Fatal("removed an active explicit gacha's reward pool")
	}
	if !bytes.Equal(rotationBefore, export("gacha")) || !bytes.Equal(before, export("gacha-rules")) {
		t.Fatal("rejected active-pool removal modified a catalog")
	}
	if err := catalog.ApplyGacha(ctx, "gacha", "overwrite", encode(automatic)); err != nil {
		t.Fatal(err)
	}
	profileCall(t, papi.NewGachaClient(testConfig.GrpcConn).List, &papi.Empty{})
}
