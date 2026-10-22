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
)

func TestMasterResetOnlineCLIScenario(t *testing.T) {
	serverBinary := os.Getenv("MAKINO_TEST_SERVER_BINARY")
	if serverBinary == "" || os.Getenv("MAKINO_TEST_CLI_BINARY") == "" {
		t.Skip("requires built server and CLI binaries")
	}
	original := setupMasterReset(t)
	customizeMasterReset(t, "Card", "LoveLevel")
	backupMakinoCatalogCollections(t, sname.NoticeCatalog, sname.ShopCatalog, sname.ExchangeCatalog, sname.GachaCatalog, sname.GachaRules)
	if err := os.Mkdir(config.GetManifestDir(), 0700); err != nil {
		t.Fatal(err)
	}
	writeMasterOfficialManifest(t, original)
	// The server only checks this revision during startup. Avoid loading the
	// large real asset manifest or accidentally updating unrelated asset data.
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
		t.Fatalf("invalid test HTTP URL: %v", err)
	}
	addresses := []string{testConfig.GrpcFqdn, httpURL.Host}
	for _, address := range addresses {
		host, _, err := net.SplitHostPort(address)
		if err != nil || (host != "localhost" && !net.ParseIP(host).IsLoopback()) {
			t.Fatal("reset scenario requires loopback endpoints")
		}
		if conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond); err == nil {
			conn.Close()
			t.Fatalf("test endpoint %s is occupied; existing server was not touched", address)
		}
	}
	repoDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	server := startMakinoTestServer(t, serverBinary, repoDir, filepath.Join(t.TempDir(), "reset-server.log"), addresses)
	t.Cleanup(func() { server.stop(t); testConfig.HttpClient.CloseIdleConnections() })
	for _, selection := range [][]string{{"--type", "Card"}, {"--all"}} {
		stdout, stderr, err := runMasterStdoutCLI(t, append([]string{"master", "reset"}, selection...)...)
		if err != nil {
			t.Fatalf("online reset failed: %v, %s", err, stderr)
		}
		if strings.Contains(stderr, "publishing locally") {
			t.Fatal("reset bypassed the running server")
		}
		var job masterdata.ImportJob
		if err := json.NewDecoder(masterExportOpen(t, stdout)).Decode(&job); err != nil || !job.Request.Reset {
			t.Fatalf("online reset did not return its job: %v", err)
		}
		deadline := time.Now().Add(20 * time.Second)
		for {
			stdout, stderr, err = runMasterStdoutCLI(t, "master", "status", "--id", job.ID)
			if err != nil {
				t.Fatalf("online reset status failed: %v, %s", err, stderr)
			}
			if err := json.NewDecoder(masterExportOpen(t, stdout)).Decode(&job); err != nil {
				t.Fatal(err)
			}
			if job.State == "complete" {
				break
			}
			if job.State == "failed" || job.State == "cleanup_pending" || time.Now().After(deadline) {
				t.Fatalf("online reset ended in %s: %s", job.State, job.Error)
			}
			time.Sleep(20 * time.Millisecond)
		}
		if !job.Committed {
			t.Fatal("online reset did not commit")
		}
	}
	if err := masterdata.RefreshSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertMasterResetState(t, original, "Original", 10)
	assertNoMasterOfflineStaging(t)
}
