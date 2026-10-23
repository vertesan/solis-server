package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	mversion "github.com/vertesan/solis-server/internal/db/store/opmaster/version"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

// The CLI owns publication when no game server is running. Only temporary raw
// files are replaced, and all affected database records are restored afterward.
func TestMasterOfflineLifecycleScenario(t *testing.T) {
	serverBinary, cliBinary := os.Getenv("MAKINO_TEST_SERVER_BINARY"), os.Getenv("MAKINO_TEST_CLI_BINARY")
	if os.Getenv("MAKINO_TEST_MASTER_OFFLINE") != "1" || serverBinary == "" || cliBinary == "" {
		t.Skip("requires MAKINO_TEST_MASTER_OFFLINE=1 and built server/CLI binaries")
	}
	if testConfig.Env != Dev {
		t.Fatal("offline master publication requires development configuration")
	}
	for _, binary := range []string{serverBinary, cliBinary} {
		info, err := os.Stat(binary)
		if err != nil || !filepath.IsAbs(binary) || !info.Mode().IsRegular() {
			t.Fatalf("unavailable absolute binary path %q: %v", binary, err)
		}
	}
	httpURL, err := url.Parse(testConfig.HttpUrl)
	if err != nil || httpURL.Host == "" {
		t.Fatalf("invalid HTTP test URL: %v", err)
	}
	addresses := []string{testConfig.GrpcFqdn, httpURL.Host}
	for _, address := range addresses {
		host, _, err := net.SplitHostPort(address)
		if err != nil || (host != "localhost" && !net.ParseIP(host).IsLoopback()) {
			t.Fatalf("offline lifecycle refuses non-loopback address %q", address)
		}
		if connection, err := net.DialTimeout("tcp", address, 200*time.Millisecond); err == nil {
			connection.Close()
			t.Fatalf("%s is occupied; the existing server was not touched", address)
		}
	}
	ctx := context.Background()
	profile, err := opsolis.FindServerProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	packs, err := opsolis.GetAllMasterTagPack(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].Type < packs[j].Type })
	backupMakinoCatalogCollections(t, sname.ServerProfile, sname.MasterTagPack, sname.MasterTagPackOrig, sname.MasterImportJob,
		sname.NoticeCatalog, sname.ShopCatalog, sname.ExchangeCatalog, sname.GachaCatalog, sname.GachaRules)
	backupMasterOfficialCollections(t, "Card", "_version")
	originalResource := config.GetResourceDir()
	resource := t.TempDir()
	for _, directory := range []string{"db", "manifest"} {
		if err := os.Mkdir(filepath.Join(resource, directory), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, pack := range packs {
		copyMasterTestFile(t, filepath.Join(originalResource, "db", pack.FileName), filepath.Join(resource, "db", pack.FileName))
	}
	entries, err := os.ReadDir(filepath.Join(originalResource, "manifest"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			copyMasterTestFile(t, filepath.Join(originalResource, "manifest", entry.Name()), filepath.Join(resource, "manifest", entry.Name()))
		}
	}
	t.Setenv("SOLIS_RESOURCE_DIR", resource)
	// A matching fixture baseline isolates this test from whether the local
	// administrator has already performed the manual first-baseline update.
	original := &ptransaction.MasterTag{Version: profile.MasterVersion, MasterTagPacks: packs}
	if err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		// Jobs were snapshotted above. Real pending jobs must not be recovered
		// against this test's temporary resource directory; restore them later.
		if _, err := mongo.SessionFromContext(ctx).Client().Database("solis").Collection(sname.MasterImportJob).DeleteMany(ctx, bson.M{}); err != nil {
			return nil, err
		}
		return nil, opsolis.ReplaceOfficialMasterTag(ctx, original)
	}); err != nil {
		t.Fatal(err)
	}
	writeMasterOfficialManifest(t, original)
	repoDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	var process *makinoTestServerProcess
	t.Cleanup(func() {
		if process != nil {
			process.stop(t)
		}
		testConfig.HttpClient.CloseIdleConnections()
	})
	// Offline commands must not require the administrator token. The isolated
	// working directory prevents .env from restoring it or the real raw paths.
	command := func(authenticated bool, args ...string) ([]byte, string, error) {
		t.Helper()
		commandCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(commandCtx, cliBinary, args...)
		for _, value := range os.Environ() {
			if authenticated || !strings.HasPrefix(value, "SOLIS_ADMIN_AUTH=") {
				cmd.Env = append(cmd.Env, value)
			}
		}
		if !authenticated {
			cmd.Env = append(cmd.Env, "SOLIS_ADMIN_AUTH=")
		}
		isolateTestCommand(t, cmd, repoDir)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		return stdout.Bytes(), stderr.String(), err
	}
	succeed := func(args ...string) []byte {
		t.Helper()
		stdout, stderr, err := command(false, args...)
		if err != nil {
			t.Fatalf("offline master command failed: %v\n%s", err, stderr)
		}
		return stdout
	}
	reject := func(authenticated bool, message string, args ...string) {
		t.Helper()
		_, stderr, err := command(authenticated, args...)
		if err == nil || !strings.Contains(stderr, message) {
			t.Fatalf("expected rejected command containing %q; got %v, %s", message, err, stderr)
		}
	}
	cardFile := filepath.Join(resource, "Card.yaml")
	succeed("master", "export", "--type", "Card", "--file", cardFile)
	initialJobs := masterOfflineJobCount(t)
	if result := succeed("master", "import", "--file", cardFile); !bytes.Contains(result, []byte("0 changed pack(s)")) {
		t.Fatalf("unchanged offline import was not skipped: %s", result)
	}
	assertNoMasterOfflineStaging(t)
	if masterOfflineJobCount(t) != initialJobs {
		t.Fatal("unchanged offline import created a publication job")
	}
	cardID := editMasterCardYAML(t, cardFile, "Makino offline publication test")
	owner, err := masterdata.AcquireOwnership()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	// An offline owner excludes another direct publisher and a server startup,
	// even though neither gameplay listener is currently running. A busy lock
	// selects HTTP; a connection error must never trigger an unsafe direct write.
	reject(false, "SOLIS_ADMIN_AUTH", "master", "import", "--file", cardFile)
	unused, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	unavailableURL := "http://" + unused.Addr().String()
	if err := unused.Close(); err != nil {
		t.Fatal(err)
	}
	reject(true, "admin request failed", "--server", unavailableURL, "master", "import", "--file", cardFile)
	// HTTP failures intentionally retain staging for ambiguous acceptance.
	// This test knows that no server listened and no job was accepted, so its
	// temporary artifacts can be removed after checking durable state below.
	if current := masterPublicationActive(t); !proto.Equal(current, original) {
		t.Fatal("failed HTTP request fell back to direct publication")
	}
	serverCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	blocked := exec.CommandContext(serverCtx, serverBinary)
	isolateTestCommand(t, blocked, repoDir)
	output, blockedErr := blocked.CombinedOutput()
	cancel()
	if blockedErr == nil || !bytes.Contains(output, []byte(masterdata.ErrOwnershipBusy.Error())) {
		t.Fatalf("server did not reject offline ownership: %v", blockedErr)
	}
	if masterOfflineJobCount(t) != initialJobs {
		t.Fatal("rejected owners created publication jobs")
	}
	staged, err := os.ReadDir(filepath.Join(config.GetDBDir(), ".master-staging"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, entry := range staged {
		if err := masterdata.RemoveStaging(entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	assertNoMasterOfflineStaging(t)
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if result := succeed("master", "import", "--file", cardFile, "--dry-run"); !bytes.Contains(result, []byte("1 changed pack(s)")) {
		t.Fatalf("offline dry run did not detect customized Card: %s", result)
	}
	assertNoMasterOfflineStaging(t)
	if masterOfflineJobCount(t) != initialJobs {
		t.Fatal("offline dry run created a publication job")
	}
	var completed masterdata.ImportJob
	if err := json.Unmarshal(succeed("master", "import", "--file", cardFile), &completed); err != nil {
		t.Fatal(err)
	}
	if completed.ID == "" || completed.State != "complete" || !completed.Committed || completed.Version == profile.MasterVersion {
		t.Fatalf("offline import returned before successful publication: state=%s committed=%v", completed.State, completed.Committed)
	}
	var status masterdata.ImportJob
	if err := json.Unmarshal(succeed("master", "status", "--id", completed.ID), &status); err != nil || !reflect.DeepEqual(status, completed) {
		t.Fatalf("offline status differs from committed publication: %v", err)
	}
	assertNoMasterOfflineStaging(t)
	active := masterPublicationActive(t)
	assertActive := func() {
		t.Helper()
		current := masterPublicationActive(t)
		if !proto.Equal(current, active) {
			t.Fatal("rejected command or restart changed the committed active manifest")
		}
		version, err := mversion.GetMasterVersion(ctx)
		if err != nil || version != completed.Version {
			t.Fatalf("master._version differs from committed publication: %v", err)
		}
		card, err := opmaster.FindCard(ctx, cardID)
		if err != nil || card.GetName() != "Makino offline publication test" {
			t.Fatalf("offline customized protobuf was not preserved: %v", err)
		}
		originals, err := opsolis.GetAllOriginalMasterTagPack(ctx)
		if err != nil {
			t.Fatal(err)
		}
		currentProfile, err := opsolis.FindServerProfile(ctx)
		if err != nil || currentProfile.MasterVersionOrig != original.Version || len(originals) != len(packs) {
			t.Fatalf("offline import changed the original baseline: %v", err)
		}
		byType := make(map[string]*ptransaction.MasterTagPack, len(originals))
		for _, pack := range originals {
			byType[pack.Type] = pack
		}
		for _, pack := range packs {
			if !proto.Equal(pack, byType[pack.Type]) {
				t.Fatalf("offline import changed original %s manifest", pack.Type)
			}
			_, err := os.Stat(filepath.Join(config.GetDBDir(), pack.FileName))
			if err != nil {
				t.Fatalf("original %s file must remain available for reset: %v", pack.Type, err)
			}
		}
	}
	assertActive()
	// Both validation errors must leave the completed transaction and all raw
	// files intact. A stale YAML cannot silently replace a newer publication.
	reject(false, "stale", "master", "import", "--file", cardFile)
	invalid := filepath.Join(resource, "Invalid.yaml")
	if err := os.WriteFile(invalid, []byte("formatVersion: [\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := command(false, "master", "import", "--file", invalid); err == nil {
		t.Fatal("malformed offline YAML was accepted")
	}
	if masterOfflineJobCount(t) != initialJobs+1 {
		t.Fatal("invalid offline imports created publication jobs")
	}
	assertNoMasterOfflineStaging(t)
	assertActive()
	// Startup must load the customized version, not replay official master
	// rows. Check the actual server's snapshot through its existing RPC.
	process = startMakinoTestServer(t, serverBinary, repoDir, filepath.Join(resource, "server.log"), addresses)
	setupTestUser(t)
	tag := profileCall(t, papi.NewMasterClient(testConfig.GrpcConn).Get, &papi.Empty{}).GetMasterTag()
	if !proto.Equal(tag, active) {
		t.Fatal("server startup did not expose the offline-published master snapshot")
	}
	assertActive()
	currentFile := filepath.Join(resource, "CurrentCard.yaml")
	succeed("master", "export", "--type", "Card", "--file", currentFile)
	editMasterCardYAML(t, currentFile, "Makino online publication test")
	reject(false, "SOLIS_ADMIN_AUTH", "master", "import", "--file", currentFile)
	if masterOfflineJobCount(t) != initialJobs+1 {
		t.Fatal("unauthenticated attempt beside a server created a publication job")
	}
	assertNoMasterOfflineStaging(t)
	assertActive()
	// The same command automatically selects authenticated asynchronous HTTP
	// publication while the server owns the resources.
	onlineOutput, stderr, err := command(true, "master", "import", "--file", currentFile)
	if err != nil {
		t.Fatalf("automatic online publication failed: %v\n%s", err, stderr)
	}
	var online masterdata.ImportJob
	if err := json.Unmarshal(onlineOutput, &online); err != nil || online.ID == "" {
		t.Fatalf("online publication did not return a job: %v", err)
	}
	deadline := time.Now().Add(time.Minute)
	for {
		statusOutput, stderr, err := command(true, "master", "status", "--id", online.ID)
		if err != nil {
			t.Fatalf("online status failed: %v\n%s", err, stderr)
		}
		if err := json.Unmarshal(statusOutput, &online); err != nil {
			t.Fatal(err)
		}
		if online.State == "complete" {
			break
		}
		if online.State == "failed" || online.State == "cleanup_pending" || time.Now().After(deadline) {
			t.Fatalf("online publication did not complete: %s (%s)", online.State, online.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !online.Committed || online.Version == completed.Version {
		t.Fatal("automatic online publication did not commit a new version")
	}
	if card, err := opmaster.FindCard(ctx, cardID); err != nil || card.GetName() != "Makino online publication test" {
		t.Fatalf("automatic online publication did not replace Card: %v", err)
	}
	tag = profileCall(t, papi.NewMasterClient(testConfig.GrpcConn).Get, &papi.Empty{}).GetMasterTag()
	if tag.GetVersion() != online.Version {
		t.Fatal("automatic online publication did not refresh the server snapshot")
	}
	assertNoMasterOfflineStaging(t)
}

func masterOfflineJobCount(t *testing.T) int64 {
	t.Helper()
	var count int64
	err := transaction.WithTransaction(context.Background(), func(ctx context.Context) (any, error) {
		var err error
		count, err = mongo.SessionFromContext(ctx).Client().Database("solis").Collection(sname.MasterImportJob).CountDocuments(ctx, bson.M{})
		return nil, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func assertNoMasterOfflineStaging(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(config.GetDBDir(), ".master-staging"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("offline command leaked staged publication files")
	}
}
