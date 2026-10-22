package test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	mversion "github.com/vertesan/solis-server/internal/db/store/opmaster/version"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/crypto"
	"google.golang.org/protobuf/proto"
)

func TestMasterPublicationRecoveryScenario(t *testing.T) {
	if os.Getenv("MAKINO_TEST_MASTER_RECOVERY") != "1" {
		t.Skip("set MAKINO_TEST_MASTER_RECOVERY=1 with the game server stopped")
	}
	if testConfig.Env != Dev {
		t.Fatal("master publication scenario requires local development configuration")
	}
	if connection, err := net.DialTimeout("tcp", testConfig.GrpcFqdn, 200*time.Millisecond); err == nil {
		connection.Close()
		t.Fatal("stop the game server before running this isolated coordinator scenario")
	}
	// Registered first so snapshot refresh runs after all database restoration.
	t.Cleanup(func() {
		if err := masterdata.RefreshSnapshot(context.Background()); err != nil {
			t.Error(err)
		}
	})
	backupMakinoCatalogCollections(t, sname.ServerProfile, sname.MasterTagPack, sname.MasterTagPackOrig, sname.MasterImportJob)
	backupMasterOfficialCollections(t, "Card", "_version")
	t.Setenv("SOLIS_RESOURCE_DIR", t.TempDir())
	for _, directory := range []string{config.GetDBDir(), config.GetManifestDir()} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	original := masterOfficialFixture(t, "publication-original", "Original")
	writeMasterOfficialManifest(t, original)
	if err := opsolis.ReplaceOfficialMasterTag(ctx, original); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckMasterDataUpdate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := masterdata.RefreshSnapshot(ctx); err != nil {
		t.Fatal(err)
	}
	serverCtx, cancel := context.WithCancel(ctx)
	coordinator := &masterdata.Coordinator{}
	coordinator.Start(serverCtx)
	t.Cleanup(func() { cancel(); coordinator.Wait() })

	// An existing gameplay/admin reader must finish before the version changes;
	// new readers get maintenance instead of accumulating behind a writer lock.
	leave, admitted := coordinator.Gate.Enter()
	if !admitted {
		t.Fatal("fresh publication gate rejected a request")
	}
	t.Cleanup(leave)
	request := stageMasterPublication(t, original, "publication-custom", "Custom")
	accepted, err := coordinator.Submit(ctx, *request)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		newLeave, admitted := coordinator.Gate.Enter()
		if !admitted {
			break
		}
		newLeave()
		if time.Now().After(deadline) {
			t.Fatal("publication never paused the request gate")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if version, err := mversion.GetMasterVersion(ctx); err != nil || version != original.Version {
		t.Fatalf("publication ran before the admitted request drained: %q (%v)", version, err)
	}
	if masterdata.Current().Version() != original.Version {
		t.Fatal("snapshot advanced before request draining")
	}
	leave()
	job := waitMasterPublicationJob(t, coordinator, accepted.ID, "complete")
	if !job.Committed || job.Version == original.Version || masterdata.Current().Version() != job.Version {
		t.Fatal("completed publication did not advance the immutable snapshot")
	}
	active := masterPublicationActive(t)
	assertMasterOfficialState(t, active, original, "Custom")
	if _, err := os.Stat(filepath.Join(config.GetDBDir(), original.MasterTagPacks[0].FileName)); err != nil {
		t.Fatalf("superseded original pack must remain available for reset: %v", err)
	}
	if leave, admitted := coordinator.Gate.Enter(); !admitted {
		t.Fatal("gate did not reopen after publication")
	} else {
		leave()
	}
	cloned := masterdata.Current().Tag()
	cloned.MasterTagPacks[0].FileName = "mutated-client-copy"
	if masterdata.Current().Tag().MasterTagPacks[0].FileName != active.MasterTagPacks[0].FileName {
		t.Fatal("snapshot was mutable through its returned protobuf")
	}
	if retried, err := coordinator.Submit(ctx, *request); err != nil || retried.ID != job.ID || !retried.Committed {
		t.Fatalf("accepted-job HTTP retry was not idempotent: %v", err)
	}

	// An invalid staged hash is rejected before pausing or replacing live data.
	badHash := stageMasterPublication(t, active, "bad-hash", "Must not publish")
	staging := filepath.Join(config.GetDBDir(), ".master-staging", badHash.ID)
	wrongName := strings.Repeat("f", 64)
	if err := os.Rename(filepath.Join(staging, badHash.Packs[0].FileName), filepath.Join(staging, wrongName)); err != nil {
		t.Fatal(err)
	}
	badHash.Packs[0].FileName = wrongName
	if _, err := coordinator.Submit(ctx, *badHash); err != nil {
		t.Fatal(err)
	}
	waitMasterPublicationJob(t, coordinator, badHash.ID, "failed")
	assertMasterOfficialState(t, active, original, "Custom")
	stale := stageMasterPublication(t, active, "stale-version", "Must not publish")
	stale.ExpectedVersion = original.Version
	if _, err := coordinator.Submit(ctx, *stale); err != nil {
		t.Fatal(err)
	}
	waitMasterPublicationJob(t, coordinator, stale.ID, "failed")
	assertMasterOfficialState(t, active, original, "Custom")

	// Simulate a crash after publishing a new encrypted file but before the
	// Mongo transaction committed. Recovery removes only that orphan file.
	queued := stageMasterPublication(t, active, "queued-crash", "Never committed")
	copyMasterPublicationFile(t, filepath.Join(config.GetDBDir(), ".master-staging", queued.ID, queued.Packs[0].FileName), filepath.Join(config.GetDBDir(), queued.Packs[0].FileName))
	queuedJob := &masterdata.ImportJob{ID: queued.ID, Request: *queued, State: "publishing", Version: "queued-crash-version"}
	if err := opsolis.InsertMasterImportJob(ctx, queuedJob); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(config.GetDBDir(), strings.Repeat("c", 64))
	if err := os.WriteFile(unrelated, []byte("unrelated file"), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted := &masterdata.Coordinator{}
	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	queuedState, err := opsolis.FindMasterImportJob(ctx, queued.ID)
	if err != nil || queuedState.Committed || queuedState.State != "failed" {
		t.Fatalf("precommit recovery did not fence the aborted publication: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.GetDBDir(), queued.Packs[0].FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("precommit recovery retained the orphan file: %v", err)
	}
	if data, err := os.ReadFile(unrelated); err != nil || string(data) != "unrelated file" {
		t.Fatal("recovery altered an unrelated raw file")
	}
	assertMasterOfficialState(t, active, original, "Custom")

	// Failed orphan cleanup must remain recoverable rather than becoming a
	// terminal failed job that the next startup will ignore.
	blocked := stageMasterPublication(t, active, "blocked-abort-cleanup", "Never committed")
	blockedPath := filepath.Join(config.GetDBDir(), blocked.Packs[0].FileName)
	if err := os.Symlink(unrelated, blockedPath); err != nil {
		t.Fatal(err)
	}
	if err := opsolis.InsertMasterImportJob(ctx, &masterdata.ImportJob{ID: blocked.ID, Request: *blocked, State: "preparing"}); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Recover(ctx); err == nil {
		t.Fatal("unsafe uncommitted orphan cleanup was accepted")
	}
	blockedState, err := opsolis.FindMasterImportJob(ctx, blocked.ID)
	if err != nil || blockedState.State != "abort_cleanup_pending" || blockedState.Committed {
		t.Fatalf("interrupted abort cleanup is not recoverable: %v", err)
	}
	if err := os.Remove(blockedPath); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	blockedState, err = opsolis.FindMasterImportJob(ctx, blocked.ID)
	if err != nil || blockedState.State != "failed" {
		t.Fatalf("repaired aborted cleanup was not completed: %v", err)
	}
	assertMasterOfficialState(t, active, original, "Custom")

	// Simulate a crash after Mongo committed, before old-file cleanup. Recovery
	// preserves the committed rows/version and completes only targeted cleanup.
	committedRequest := stageMasterPublication(t, active, "committed-crash", "Recovered")
	copyMasterPublicationFile(t, filepath.Join(config.GetDBDir(), ".master-staging", committedRequest.ID, committedRequest.Packs[0].FileName), filepath.Join(config.GetDBDir(), committedRequest.Packs[0].FileName))
	committedTag := proto.Clone(active).(*ptransaction.MasterTag)
	committedTag.Version = "committed-crash-version"
	committedTag.MasterTagPacks[0].FileName = committedRequest.Packs[0].FileName
	committedTag.MasterTagPacks[0].FileSize = int32(committedRequest.Packs[0].FileSize)
	committedJob := &masterdata.ImportJob{
		ID: committedRequest.ID, Request: *committedRequest, State: "committed", Committed: true, Version: committedTag.Version,
	}
	rows, err := crypto.DecryptMasterDbFile("Card", filepath.Join(config.GetDBDir(), committedRequest.Packs[0].FileName), committedTag.MasterTagPacks[0].CryptoKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := mversion.DeleteMasterDb(ctx, "Card"); err != nil {
			return nil, err
		}
		if err := mversion.InsertMasterDb(ctx, "Card", rows); err != nil {
			return nil, err
		}
		if err := opsolis.UpsertMasterTagPack(ctx, committedTag.MasterTagPacks[0]); err != nil {
			return nil, err
		}
		if err := opsolis.UpdateServerProfileMasterVersion(ctx, committedTag.Version); err != nil {
			return nil, err
		}
		if err := mversion.UpsertMasterVersion(ctx, committedTag.Version); err != nil {
			return nil, err
		}
		return nil, opsolis.InsertMasterImportJob(ctx, committedJob)
	}); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := masterdata.RefreshSnapshot(ctx); err != nil {
		t.Fatal(err)
	}
	assertMasterOfficialState(t, committedTag, original, "Recovered")
	completed, err := opsolis.FindMasterImportJob(ctx, committedJob.ID)
	if err != nil || !completed.Committed || completed.State != "complete" {
		t.Fatalf("committed recovery did not finish cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.GetDBDir(), active.MasterTagPacks[0].FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed recovery did not delete the superseded pack: %v", err)
	}

	// A malicious/replaced obsolete path must never delete an external file.
	// Its committed job remains cleanup_pending and is retried after repair.
	outside := filepath.Join(t.TempDir(), "keep.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkName := strings.Repeat("a", 64)
	symlinkPath := filepath.Join(config.GetDBDir(), symlinkName)
	if err := os.Symlink(outside, symlinkPath); err != nil {
		t.Fatal(err)
	}
	symlinkJob := masterPublicationCleanupJob(committedTag, symlinkName, committedTag.MasterTagPacks[0].FileName)
	if err := opsolis.InsertMasterImportJob(ctx, symlinkJob); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	pending, err := opsolis.FindMasterImportJob(ctx, symlinkJob.ID)
	if err != nil || !pending.Committed || pending.State != "cleanup_pending" {
		t.Fatalf("unsafe cleanup did not retain committed cleanup-pending state: %v", err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "keep" {
		t.Fatal("cleanup followed a symlink outside the raw database directory")
	}
	if info, err := os.Lstat(symlinkPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("cleanup unexpectedly removed/replaced the rejected symlink")
	}
	if err := os.Remove(symlinkPath); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	completed, err = opsolis.FindMasterImportJob(ctx, symlinkJob.ID)
	if err != nil || completed.State != "complete" {
		t.Fatalf("repaired cleanup was not retried: %v", err)
	}
	retainedJob := masterPublicationCleanupJob(committedTag, committedTag.MasterTagPacks[0].FileName, strings.Repeat("b", 64))
	if err := opsolis.InsertMasterImportJob(ctx, retainedJob); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(config.GetDBDir(), committedTag.MasterTagPacks[0].FileName)); err != nil {
		t.Fatalf("cleanup deleted a still-active filename: %v", err)
	}
	assertMasterOfficialState(t, committedTag, original, "Recovered")
}

func stageMasterPublication(t *testing.T, active *ptransaction.MasterTag, version, name string) *masterdata.ImportRequest {
	t.Helper()
	fixture := masterOfficialFixture(t, version, name)
	pack := fixture.MasterTagPacks[0]
	request := &masterdata.ImportRequest{
		ID: uuid.NewString(), ExpectedVersion: active.Version,
		Packs: []masterdata.StagedPack{{Type: pack.Type, BaseFileName: active.MasterTagPacks[0].FileName, FileName: pack.FileName, FileSize: int64(pack.FileSize), RowCount: 1}},
	}
	directory := filepath.Join(config.GetDBDir(), ".master-staging", request.ID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(config.GetDBDir(), pack.FileName), filepath.Join(directory, pack.FileName)); err != nil {
		t.Fatal(err)
	}
	return request
}

func masterPublicationActive(t *testing.T) *ptransaction.MasterTag {
	t.Helper()
	profile, err := opsolis.FindServerProfile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	packs, err := opsolis.GetAllMasterTagPack(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return &ptransaction.MasterTag{Version: profile.MasterVersion, MasterTagPacks: packs}
}

func waitMasterPublicationJob(t *testing.T, coordinator *masterdata.Coordinator, id, state string) *masterdata.ImportJob {
	t.Helper()
	finished := make(chan struct{})
	go func() { coordinator.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("master publication worker did not finish")
	}
	job, err := opsolis.FindMasterImportJob(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != state {
		t.Fatalf("master job state %q, expected %q: %s", job.State, state, job.Error)
	}
	return job
}

func copyMasterPublicationFile(t *testing.T, source, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func masterPublicationCleanupJob(active *ptransaction.MasterTag, oldName, newName string) *masterdata.ImportJob {
	id := uuid.NewString()
	return &masterdata.ImportJob{
		ID: id, State: "committed", Committed: true, Version: active.Version,
		Request: masterdata.ImportRequest{
			ID: id, ExpectedVersion: fmt.Sprintf("prior-%s", active.Version),
			Packs: []masterdata.StagedPack{{Type: "Card", BaseFileName: oldName, FileName: newName, FileSize: int64(active.MasterTagPacks[0].FileSize), RowCount: 1}},
		},
	}
}
