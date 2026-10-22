package test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"go.yaml.in/yaml/v3"
)

// Run with MAKINO_TEST_CLI_BINARY=/absolute/path/to/makino. Using a subprocess
// verifies argument parsing and database connect/disconnect independently of
// the test's existing connection and running game server.
func TestMakinoServerCommandScenario(t *testing.T) {
	binary := os.Getenv("MAKINO_TEST_CLI_BINARY")
	if binary == "" {
		t.Skip("set MAKINO_TEST_CLI_BINARY to exercise the compiled management tool")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("MAKINO_TEST_CLI_BINARY must be absolute")
	}
	backupMakinoCatalogCollections(t, sname.NoticeCatalog, sname.ShopCatalog, sname.ExchangeCatalog, sname.GachaCatalog, sname.GachaRules)
	setupTestUser(t)
	ctx := context.Background()
	working := t.TempDir()
	run := func(wantFailure bool, args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		command := exec.CommandContext(ctx, binary, args...)
		command.Dir = working
		var stderr bytes.Buffer
		command.Stderr = &stderr
		stdout, err := command.Output()
		if wantFailure != (err != nil) {
			t.Fatalf("makino %s: err=%v, stderr=%s", strings.Join(args, " "), err, stderr.String())
		}
		return stdout
	}
	write := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(working, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	export := func(kind string) []byte {
		t.Helper()
		return run(false, "server", kind, "--export")
	}
	kinds := []string{"notice", "shop", "exchange", "gacha", "gacha-rules"}
	beforeHelp := make(map[string][]byte)
	for _, kind := range kinds {
		beforeHelp[kind] = export(kind)
	}
	run(false, "--help")
	run(false, "server", "--help")
	for _, kind := range kinds {
		run(false, "server", kind, "--help")
		if !bytes.Equal(beforeHelp[kind], export(kind)) {
			t.Fatalf("help changed %s catalog", kind)
		}
	}
	// Defaults belong to the administrator's working tree, not the server's
	// initialization seeds. A missing directory must not erase existing data.
	run(true, "server", "notice", "--overwrite")
	if !bytes.Equal(beforeHelp["notice"], export("notice")) {
		t.Fatal("missing default input directory changed the notice catalog")
	}
	write("config/catalogs/notice/catalog.yaml", []byte("id: cli-default-notice\ntitle: Default working directory\n"))
	run(false, "server", "notice", "--overwrite")
	defaultNotice := profileCall(t, papi.NewNoticeClient(testConfig.GrpcConn).Get, &papi.NoticeGetRequest{NoticeId: "cli-default-notice"})
	if defaultNotice.GetNotice().GetTitle() != "Default working directory" {
		t.Fatal("CLI did not use config/catalogs/notice by default")
	}

	write("notice/01.yml", []byte("id: cli-notice-one\ntitle: First CLI notice\n---\nid: cli-notice-two\ntitle: Second CLI notice\n"))
	write("notice/nested/02.yaml", []byte("malfunctionNotices:\n  - id: cli-malfunction\n    title: CLI maintenance\n"))
	write("notice/ignored.txt", []byte("not yaml"))
	run(false, "server", "notice", "--overwrite", "--dir", filepath.Join(working, "notice"))
	noticeClient := papi.NewNoticeClient(testConfig.GrpcConn)
	notices := profileCall(t, noticeClient.List, &papi.Empty{})
	if len(notices.GetNotices()) != 2 || len(notices.GetMalfunctionNotices()) != 1 {
		t.Fatalf("directory import did not load all YAML documents: %v", notices)
	}
	if got := profileCall(t, noticeClient.Get, &papi.NoticeGetRequest{NoticeId: "cli-notice-two"}); got.GetNotice().GetTitle() != "Second CLI notice" {
		t.Fatalf("CLI notice not visible live: %v", got)
	}
	addFile := write("notice-add.yaml", []byte("id: cli-notice-two\ntitle: Replaced by CLI\n"))
	run(false, "server", "notice", "--add", "--file", addFile)
	if got := profileCall(t, noticeClient.Get, &papi.NoticeGetRequest{NoticeId: "cli-notice-two"}); got.GetNotice().GetTitle() != "Replaced by CLI" {
		t.Fatal("CLI add did not replace matching notice")
	}
	beforeRejected := export("notice")
	write("bad-notices/01.yaml", []byte("id: would-have-replaced-everything\n"))
	write("bad-notices/02.yaml", []byte("id: bad\nunknownField: invalid\n"))
	run(true, "server", "notice", "--overwrite", "--dir", filepath.Join(working, "bad-notices"))
	if !bytes.Equal(beforeRejected, export("notice")) {
		t.Fatal("malformed CLI batch partially modified notices")
	}
	emptyDir := filepath.Join(working, "empty")
	if err := os.Mkdir(emptyDir, 0700); err != nil {
		t.Fatal(err)
	}
	run(true, "server", "notice", "--overwrite", "--dir", emptyDir)
	if !bytes.Equal(beforeRejected, export("notice")) {
		t.Fatal("empty CLI directory cleared notices")
	}

	// Actual exported, edited-data schemas must round-trip through the binary.
	for _, kind := range []string{"notice", "shop", "exchange", "gacha-rules"} {
		before := export(kind)
		file := write("roundtrip-"+kind+".yaml", before)
		run(false, "server", kind, "--overwrite", "--file", file)
		if !bytes.Equal(before, export(kind)) {
			t.Fatalf("CLI %s export/overwrite round trip changed catalog", kind)
		}
	}
	shopList := profileCall(t, papi.NewShopClient(testConfig.GrpcConn).List, &papi.Empty{})
	exchangeList := profileCall(t, papi.NewExchangeClient(testConfig.GrpcConn).List, &papi.Empty{})
	if len(shopList.GetShops()) == 0 || len(exchangeList.GetBooths()) == 0 {
		t.Fatal("round-tripped commerce catalogs are unavailable")
	}

	gachaClient := papi.NewGachaClient(testConfig.GrpcConn)
	baseline := profileCall(t, gachaClient.List, &papi.Empty{})
	normalID := ""
	for _, entry := range baseline.GetGachas() {
		if strings.HasPrefix(entry.GetId(), "gacha-normal-") {
			normalID = entry.GetId()
			break
		}
	}
	if normalID == "" {
		t.Fatal("need one visible normal gacha for CLI roster test")
	}
	oldGacha, _ := testExpiredDiamondGacha(t)
	rosterFile := write("roster.yaml", []byte("gachaIds: ["+normalID+"]\nignoreOfficialDates: true\n"))
	run(false, "server", "gacha", "--overwrite", "--file", rosterFile)
	if got := profileCall(t, gachaClient.List, &papi.Empty{}); len(got.GetGachas()) != 1 || got.Gachas[0].GetId() != normalID {
		t.Fatalf("CLI roster did not replace automatic selection: %v", got.GetGachas())
	}
	addGachaFile := write("roster-add.yaml", []byte("gachaIds: ["+oldGacha.GetId()+"]\n"))
	run(false, "server", "gacha", "--add", "--file", addGachaFile)
	roster, err := opsolis.FindGachaCatalog(ctx)
	if err != nil || roster.UseMasterCatalog || !roster.IgnoreOfficialDates || !slices.Equal(roster.GachaIds, []string{normalID, oldGacha.GetId()}) {
		t.Fatalf("CLI append changed roster policy: %v %v", roster, err)
	}
	if got := profileCall(t, gachaClient.List, &papi.Empty{}); len(got.GetGachas()) != 2 {
		t.Fatalf("CLI did not reopen old gacha: %v", got.GetGachas())
	}
	beforeRejected = export("gacha")
	unknownGacha := write("roster-unknown.yaml", []byte("gachaIds: [does-not-exist-makino-test]\n"))
	run(true, "server", "gacha", "--overwrite", "--file", unknownGacha)
	if !bytes.Equal(beforeRejected, export("gacha")) {
		t.Fatal("unknown master gacha changed rotation")
	}
	file := write("roster-roundtrip.yaml", export("gacha"))
	run(false, "server", "gacha", "--overwrite", "--file", file)

	var rules smodel.GachaRules
	if err := yaml.Unmarshal(export("gacha-rules"), &rules); err != nil {
		t.Fatal(err)
	}
	rules.StandardCardRarityProbabilities = map[int32]int64{3: 80000, 4: 10000, 5: 10000}
	rulesYAML, err := yaml.Marshal(&rules)
	if err != nil {
		t.Fatal(err)
	}
	run(false, "server", "gacha-rules", "--overwrite", "--file", write("rules.yaml", rulesYAML))
	storedRules, err := opsolis.FindGachaRules(ctx)
	if err != nil || !reflect.DeepEqual(storedRules, &rules) {
		t.Fatalf("CLI rules not saved: %v", err)
	}
	probabilities := profileCall(t, gachaClient.Probability, &papi.GachaProbabilityRequest{GachaId: normalID})
	for _, tier := range probabilities.GetRarityProbabilityInfos() {
		if int64(tier.GetProbability()) != rules.StandardCardRarityProbabilities[tier.GetRarity()] {
			t.Fatalf("CLI rule edit not applied live: %v", tier)
		}
	}
	beforeRejected = export("gacha-rules")
	run(true, "server", "gacha-rules", "--clear")
	if !bytes.Equal(beforeRejected, export("gacha-rules")) {
		t.Fatal("unsupported rules clear modified database")
	}

	for _, kind := range []string{"notice", "shop", "exchange", "gacha"} {
		run(false, "server", kind, "--clear")
	}
	if got := profileCall(t, noticeClient.List, &papi.Empty{}); len(got.GetNotices())+len(got.GetMalfunctionNotices())+len(got.GetPrNotices()) != 0 {
		t.Fatal("CLI clear left notices")
	}
	if got := profileCall(t, papi.NewShopClient(testConfig.GrpcConn).List, &papi.Empty{}); len(got.GetShops()) != 0 {
		t.Fatal("CLI clear left shops")
	}
	if got := profileCall(t, papi.NewExchangeClient(testConfig.GrpcConn).List, &papi.Empty{}); len(got.GetBooths()) != 0 {
		t.Fatal("CLI clear left exchanges")
	}
	if got := profileCall(t, gachaClient.List, &papi.Empty{}); len(got.GetGachas()) != 0 {
		t.Fatal("CLI clear fell back to master gachas")
	}
	for _, kind := range []string{"notice", "shop", "exchange", "gacha"} {
		cleared := export(kind)
		// A second command must connect only, not seed the cleared catalog.
		if !bytes.Equal(cleared, export(kind)) {
			t.Fatalf("CLI export repopulated cleared %s", kind)
		}
	}
	shops, err := opsolis.FindAllShopCatalogEntries(ctx)
	if err != nil || len(shops) != 0 {
		t.Fatalf("shop clear not persisted: %v", err)
	}
	booths, err := opsolis.FindAllExchangeBooths(ctx)
	if err != nil || len(booths) != 0 {
		t.Fatalf("exchange clear not persisted: %v", err)
	}
	roster, err = opsolis.FindGachaCatalog(ctx)
	if err != nil || roster.UseMasterCatalog || len(roster.GachaIds) != 0 {
		t.Fatalf("gacha clear not persisted: %v", err)
	}
}
