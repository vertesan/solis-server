package test

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/utils/config"
	"google.golang.org/protobuf/proto"
)

func TestCustomMasterOnlineCLIScenario(t *testing.T) {
	serverBinary := os.Getenv("MAKINO_TEST_SERVER_BINARY")
	if serverBinary == "" || os.Getenv("MAKINO_TEST_CLI_BINARY") == "" {
		t.Skip("requires built server and CLI binaries")
	}
	original, rows := setupCustomMaster(t)
	backupMakinoCatalogCollections(t, sname.NoticeCatalog, sname.ShopCatalog, sname.ExchangeCatalog, sname.GachaCatalog, sname.GachaRules)
	if err := os.Mkdir(config.GetManifestDir(), 0700); err != nil {
		t.Fatal(err)
	}
	writeMasterOfficialManifest(t, original)
	// Keep unrelated asset data untouched without loading a full asset manifest.
	meta, err := opsolis.FindManifestMetaInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(map[string]any{"revision": meta.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.GetManifestDir(), "OctoManifest.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	httpURL, err := url.Parse(testConfig.HttpUrl)
	if err != nil || httpURL.Host == "" {
		t.Fatal("invalid HTTP test URL")
	}
	addresses := []string{testConfig.GrpcFqdn, httpURL.Host}
	for _, address := range addresses {
		host, _, err := net.SplitHostPort(address)
		if err != nil || (host != "localhost" && !net.ParseIP(host).IsLoopback()) {
			t.Fatal("custom scenario requires loopback endpoints")
		}
		if conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond); err == nil {
			conn.Close()
			t.Fatalf("test endpoint %s is occupied; existing server was not touched", address)
		}
	}
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	server := startMakinoTestServer(t, serverBinary, repo, filepath.Join(t.TempDir(), "custom-server.log"), addresses)
	t.Cleanup(func() { server.stop(t); testConfig.HttpClient.CloseIdleConnections() })
	publish := func(args ...string) {
		t.Helper()
		stdout, stderr, err := runMasterStdoutCLI(t, append([]string{"custom"}, args...)...)
		if err != nil {
			t.Fatalf("online customization failed: %v, %s", err, stderr)
		}
		if strings.Contains(stderr, "publishing locally") {
			t.Fatal("customization bypassed the running server")
		}
		var job masterdata.ImportJob
		if err := json.NewDecoder(masterExportOpen(t, stdout)).Decode(&job); err != nil || job.ID == "" {
			t.Fatalf("missing custom publication job: %v", err)
		}
		deadline := time.Now().Add(20 * time.Second)
		for {
			stdout, stderr, err = runMasterStdoutCLI(t, "master", "status", "--id", job.ID)
			if err != nil {
				t.Fatalf("custom status failed: %v, %s", err, stderr)
			}
			if err := json.NewDecoder(masterExportOpen(t, stdout)).Decode(&job); err != nil {
				t.Fatal(err)
			}
			if job.State == "complete" {
				break
			}
			if job.State == "failed" || job.State == "cleanup_pending" || time.Now().After(deadline) {
				t.Fatalf("custom publication ended in %s: %s", job.State, job.Error)
			}
			time.Sleep(20 * time.Millisecond)
		}
		if !job.Committed {
			t.Fatal("custom publication did not commit")
		}
	}
	publish("home-action")
	assertCustomMasterRows(t, "HomeAction", customMasterExpectedHome(rows))
	publish("photo")
	for name, expected := range customMasterExpectedPhoto(rows) {
		assertCustomMasterRows(t, name, expected)
	}
	publish("home-action", "--reset")
	assertCustomMasterRows(t, "HomeAction", rows["HomeAction"])
	for name, expected := range customMasterExpectedPhoto(rows) {
		assertCustomMasterRows(t, name, expected)
	}
	publish("photo", "--reset")
	for name, expected := range rows {
		assertCustomMasterRows(t, name, expected)
	}
	if !proto.Equal(masterPublicationActive(t), original) {
		t.Fatal("online template resets did not restore the original manifest")
	}
	assertCustomMasterSources(t, original, rows)
	assertNoMasterOfflineStaging(t)
}
