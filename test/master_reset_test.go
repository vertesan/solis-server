package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	mversion "github.com/vertesan/solis-server/internal/db/store/opmaster/version"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/crypto"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

// These scenarios replace only two small synthetic packs. All database state
// is restored, and raw-file deletion is confined to the temporary resource tree.
func setupMasterReset(t *testing.T) *ptransaction.MasterTag {
	t.Helper()
	if os.Getenv("MAKINO_TEST_MASTER_RESET") != "1" {
		t.Skip("set MAKINO_TEST_MASTER_RESET=1 with the game server stopped")
	}
	if testConfig.Env != Dev {
		t.Fatal("master reset scenarios require local development configuration")
	}
	if conn, err := net.DialTimeout("tcp", testConfig.GrpcFqdn, 200*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("stop the game server before running isolated reset scenarios")
	}
	t.Cleanup(func() {
		if err := masterdata.RefreshSnapshot(context.Background()); err != nil {
			t.Error(err)
		}
	})
	backupMakinoCatalogCollections(t, sname.ServerProfile, sname.MasterTagPack, sname.MasterTagPackOrig, sname.MasterImportJob)
	backupMasterOfficialCollections(t, "Card", "LoveLevel", "_version")
	t.Setenv("SOLIS_RESOURCE_DIR", t.TempDir())
	if err := os.Mkdir(config.GetDBDir(), 0700); err != nil {
		t.Fatal(err)
	}
	original := masterOfficialFixture(t, "reset-original", "Original")
	original.MasterTagPacks = append(original.MasterTagPacks, masterPrepareLoveFixture(t))
	ctx := context.Background()
	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := mongo.SessionFromContext(ctx).Client().Database("solis").Collection(sname.MasterImportJob).DeleteMany(ctx, bson.M{}); err != nil {
			return nil, err
		}
		if err := opsolis.ReplaceOfficialMasterTag(ctx, original); err != nil {
			return nil, err
		}
		for _, pack := range original.MasterTagPacks {
			rows, err := crypto.DecryptMasterDbFile(pack.Type, filepath.Join(config.GetDBDir(), pack.FileName), pack.CryptoKey)
			if err != nil {
				return nil, err
			}
			if err := mversion.DeleteMasterDb(ctx, pack.Type); err != nil {
				return nil, err
			}
			if err := mversion.InsertMasterDb(ctx, pack.Type, rows); err != nil {
				return nil, err
			}
		}
		return nil, mversion.UpsertMasterVersion(ctx, original.Version)
	}); err != nil {
		t.Fatal(err)
	}
	if err := masterdata.RefreshSnapshot(ctx); err != nil {
		t.Fatal(err)
	}
	return original
}

func customizeMasterReset(t *testing.T, types ...string) *ptransaction.MasterTag {
	t.Helper()
	ctx := context.Background()
	active := masterPublicationActive(t)
	var files []string
	for _, name := range types {
		var pack *ptransaction.MasterTagPack
		for _, candidate := range active.MasterTagPacks {
			if candidate.Type == name {
				pack = candidate
			}
		}
		if pack == nil {
			t.Fatalf("missing fixture pack %s", name)
		}
		data := "data: {id: master-update-fixture, name: Customized}\n---\ndata: {id: additional-custom-card, name: Added}\n"
		if name == "LoveLevel" {
			data = "data: {loveId: love-fixture, level: 1, requiredExp: 99}\n---\ndata: {loveId: love-fixture, level: 2, requiredExp: 100}\n"
		}
		source := fmt.Sprintf("formatVersion: 1\ntype: %s\nbaseVersion: %q\nbaseFileName: %q\n---\n%s", name, active.Version, pack.FileName, data)
		file := filepath.Join(t.TempDir(), name+".yaml")
		if err := os.WriteFile(file, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	request, err := masterdata.PrepareImport(ctx, masterdata.ImportOptions{Files: files})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := masterdata.AcquireOwnership()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if job, err := owner.Publish(ctx, *request); err != nil || !job.Committed || job.State != "complete" {
		t.Fatalf("custom fixture publication failed: %v", err)
	}
	return masterPublicationActive(t)
}

func assertMasterResetState(t *testing.T, original *ptransaction.MasterTag, cardName string, loveExp int64) {
	t.Helper()
	ctx := context.Background()
	profile, err := opsolis.FindServerProfile(ctx)
	if err != nil || profile.MasterVersionOrig != original.Version {
		t.Fatalf("reset altered original version: %v", err)
	}
	packs, err := opsolis.GetAllOriginalMasterTagPack(ctx)
	if err != nil || len(packs) != len(original.MasterTagPacks) {
		t.Fatalf("reset altered original manifest count: %v", err)
	}
	for _, want := range original.MasterTagPacks {
		found := false
		for _, got := range packs {
			found = found || proto.Equal(got, want)
		}
		if !found {
			t.Fatalf("reset altered original %s manifest", want.Type)
		}
		if _, err := os.Stat(filepath.Join(config.GetDBDir(), want.FileName)); err != nil {
			t.Fatalf("original %s file was deleted: %v", want.Type, err)
		}
	}
	version, err := mversion.GetMasterVersion(ctx)
	if err != nil || version != profile.MasterVersion || masterdata.Current().Version() != version {
		t.Fatalf("reset versions/snapshot disagree: %v", err)
	}
	card, err := opmaster.FindCard(ctx, "master-update-fixture")
	if err != nil || card.GetName() != cardName {
		t.Fatalf("unexpected Card after reset: %v", err)
	}
	levels, err := opmaster.FindLoveLevels(ctx, "love-fixture")
	found := false
	for _, level := range levels {
		if level.Level == 1 && level.RequiredExp == loveExp {
			found = true
		}
	}
	if err != nil || !found {
		t.Fatalf("unexpected composite-key LoveLevel after reset: %v", err)
	}
}

func TestMasterResetScenario(t *testing.T) {
	original := setupMasterReset(t)
	custom := customizeMasterReset(t, "Card", "LoveLevel")
	assertMasterResetState(t, original, "Customized", 99)
	ctx := context.Background()
	request, err := masterdata.PrepareReset(ctx, masterdata.ResetOptions{Type: "Card"})
	if err != nil || !request.Reset || len(request.Packs) != 1 || request.Packs[0].Type != "Card" {
		t.Fatalf("single-pack reset preparation failed: %v", err)
	}
	// A reset uses the same online request gate and immutable snapshot as import.
	coordinator := &masterdata.Coordinator{}
	coordinator.Start(ctx)
	t.Cleanup(coordinator.Wait)
	leave, ok := coordinator.Gate.Enter()
	if !ok {
		t.Fatal("new coordinator rejected its first request")
	}
	t.Cleanup(leave)
	accepted, err := coordinator.Submit(ctx, *request)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		release, admitted := coordinator.Gate.Enter()
		if !admitted {
			break
		}
		release()
		if time.Now().After(deadline) {
			t.Fatal("reset never paused gameplay")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if masterdata.Current().Version() != custom.Version {
		t.Fatal("reset published before the active request finished")
	}
	leave()
	job := waitMasterPublicationJob(t, coordinator, accepted.ID, "complete")
	if !job.Committed || job.Version == custom.Version || job.Version == original.Version {
		t.Fatal("partial reset did not publish a distinct customized version")
	}
	assertMasterResetState(t, original, "Original", 99)
	for _, pack := range custom.MasterTagPacks {
		_, err := os.Stat(filepath.Join(config.GetDBDir(), pack.FileName))
		if pack.Type == "Card" && !errors.Is(err, os.ErrNotExist) {
			t.Fatal("reset did not delete superseded custom Card file")
		}
		if pack.Type == "LoveLevel" && err != nil {
			t.Fatal("partial reset deleted unselected customized LoveLevel")
		}
	}
	if _, err := opmaster.FindCard(ctx, "additional-custom-card"); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("reset retained added custom row: %v", err)
	}
	// A prepared reset becomes stale if another publication changes the version.
	stale, err := coordinator.Submit(ctx, masterdata.ImportRequest{ID: uuid.NewString(), ExpectedVersion: custom.Version, Packs: request.Packs, Reset: true})
	if err != nil {
		t.Fatal(err)
	}
	waitMasterPublicationJob(t, coordinator, stale.ID, "failed")
	assertMasterResetState(t, original, "Original", 99)
	all, err := masterdata.PrepareReset(ctx, masterdata.ResetOptions{All: true})
	if err != nil || len(all.Packs) != 1 || all.Packs[0].Type != "LoveLevel" {
		t.Fatalf("all reset failed to skip already-original Card: %v", err)
	}
	accepted, err = coordinator.Submit(ctx, *all)
	if err != nil {
		t.Fatal(err)
	}
	job = waitMasterPublicationJob(t, coordinator, accepted.ID, "complete")
	if job.Version != original.Version {
		t.Fatal("full reset did not restore the original master version")
	}
	assertMasterResetState(t, original, "Original", 10)
	for _, pack := range custom.MasterTagPacks {
		if _, err := os.Stat(filepath.Join(config.GetDBDir(), pack.FileName)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("full reset retained superseded custom %s", pack.Type)
		}
	}
	repeat, err := masterdata.PrepareReset(ctx, masterdata.ResetOptions{All: true})
	if err != nil || len(repeat.Packs) != 0 {
		t.Fatalf("repeated reset was not a no-op: %v", err)
	}
	assertNoMasterOfflineStaging(t)
}

func TestMasterResetDownloadAndRecoveryScenario(t *testing.T) {
	original := setupMasterReset(t)
	ctx := context.Background()
	card := original.MasterTagPacks[0]
	path := filepath.Join(config.GetDBDir(), card.FileName)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Official filenames need not be the hash of their encrypted contents.
	card.FileName = strings.Repeat("d", 64)
	newPath := filepath.Join(config.GetDBDir(), card.FileName)
	if err := os.Rename(path, newPath); err != nil {
		t.Fatal(err)
	}
	path = newPath
	var downloads atomic.Int32
	var truncated atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		if truncated.Load() {
			_, _ = w.Write(contents[:len(contents)/2])
			return
		}
		_, _ = w.Write(contents)
	}))
	defer server.Close()
	card.DownloadUrl = server.URL + "/" + card.FileName
	if err := opsolis.ReplaceOfficialMasterTag(ctx, original); err != nil {
		t.Fatal(err)
	}
	custom := customizeMasterReset(t, "Card")
	// Older releases deleted original files; the saved official URL recovers it.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	request, err := masterdata.PrepareReset(ctx, masterdata.ResetOptions{Type: "Card"})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(recovered, contents) || downloads.Load() != 1 {
		t.Fatalf("missing original was not recovered exactly once: %v", err)
	}
	// A precommit reset interruption must never treat the retained original file
	// as an orphan customization and delete it during recovery.
	interrupted := &masterdata.ImportJob{ID: request.ID, Request: *request, State: "publishing", Version: "never-published-reset"}
	if err := opsolis.InsertMasterImportJob(ctx, interrupted); err != nil {
		t.Fatal(err)
	}
	if err := (&masterdata.Coordinator{}).Recover(ctx); err != nil {
		t.Fatal(err)
	}
	assertMasterResetState(t, original, "Customized", 10)
	if !proto.Equal(custom, masterPublicationActive(t)) {
		t.Fatal("precommit reset recovery changed active manifests")
	}
	// Corrupt retained originals must fail validation without replacing data or
	// being silently overwritten by downloads.
	if err := os.WriteFile(path, bytes.Repeat([]byte{0}, len(contents)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := masterdata.PrepareReset(ctx, masterdata.ResetOptions{All: true}); err == nil {
		t.Fatal("corrupt original was accepted for reset")
	}
	if !proto.Equal(custom, masterPublicationActive(t)) || downloads.Load() != 1 {
		t.Fatal("failed reset changed active state or overwrote an existing original")
	}
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
	assertMasterResetState(t, original, "Customized", 10)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	truncated.Store(true)
	if _, err := masterdata.PrepareReset(ctx, masterdata.ResetOptions{All: true}); err == nil {
		t.Fatal("incomplete original download was accepted")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("incomplete download was published as an original file")
	}
	entries, err := os.ReadDir(config.GetDBDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".master-original-") {
			t.Fatal("failed original download leaked a temporary file")
		}
	}
	if !proto.Equal(custom, masterPublicationActive(t)) {
		t.Fatal("failed original download changed active state")
	}
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
	assertMasterResetState(t, original, "Customized", 10)
}

func TestMasterResetCLIScenario(t *testing.T) {
	if os.Getenv("MAKINO_TEST_CLI_BINARY") == "" {
		t.Skip("requires MAKINO_TEST_CLI_BINARY")
	}
	original := setupMasterReset(t)
	custom := customizeMasterReset(t, "Card", "LoveLevel")
	t.Setenv("SOLIS_ADMIN_AUTH", "") // Offline reset does not require HTTP credentials.
	stdout, stderr, err := runMasterStdoutCLI(t, "master", "reset", "--all", "--dry-run")
	if err != nil || !strings.Contains(masterExportPrefix(t, stdout), "2 changed pack(s)") {
		t.Fatalf("reset dry-run failed: %v, %s", err, stderr)
	}
	if !proto.Equal(custom, masterPublicationActive(t)) {
		t.Fatal("reset dry-run changed active manifest")
	}
	for _, args := range [][]string{{"--type", "Card"}, {"--all"}} {
		stdout, stderr, err = runMasterStdoutCLI(t, append([]string{"master", "reset"}, args...)...)
		if err != nil {
			t.Fatalf("offline reset CLI failed: %v, %s", err, stderr)
		}
		var job masterdata.ImportJob
		if err := json.NewDecoder(masterExportOpen(t, stdout)).Decode(&job); err != nil || !job.Committed || job.State != "complete" || !job.Request.Reset {
			t.Fatalf("CLI reset did not return a completed reset publication: %v", err)
		}
	}
	if err := masterdata.RefreshSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertMasterResetState(t, original, "Original", 10)
	stdout, stderr, err = runMasterStdoutCLI(t, "master", "reset", "--all")
	if err != nil || !strings.Contains(masterExportPrefix(t, stdout), "0 changed pack(s)") {
		t.Fatalf("repeated CLI reset was not a no-op: %v, %s", err, stderr)
	}
	assertNoMasterOfflineStaging(t)
}

func TestMasterResetCLIValidation(t *testing.T) {
	t.Setenv("SOLIS_DB_USERNAME", "")
	for _, args := range [][]string{
		{}, {"--type", "Card", "--all"}, {"--file", "Card.yaml"}, {"--dir", "config/master"}, {"--all", "--allow-empty"},
	} {
		stdout, stderr, err := runMasterStdoutCLI(t, append([]string{"master", "reset"}, args...)...)
		if err == nil || strings.Contains(stderr, "SOLIS_DB_USERNAME") || masterExportSize(t, stdout) != 0 {
			t.Fatalf("invalid reset flags did not fail before database connection: %v, %s", err, stderr)
		}
	}
}
