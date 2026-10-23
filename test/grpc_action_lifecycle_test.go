package test

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
)

// Start and stop our own server for wire-level redirect and ordinary unary
// regressions. Every scenario uses the configured integration-test account.
func TestGRPCActionLifecycleScenario(t *testing.T) {
	binary := os.Getenv("RPC_ACTION_TEST_SERVER_BINARY")
	if binary == "" {
		t.Skip("set RPC_ACTION_TEST_SERVER_BINARY to run the action-response lifecycle")
	}
	if testConfig.Env != Dev || !filepath.IsAbs(binary) {
		t.Fatal("action lifecycle requires the local development configuration and an absolute server binary")
	}
	u, err := url.Parse(testConfig.HttpUrl)
	if err != nil || u.Host == "" {
		t.Fatal("invalid test HTTP URL")
	}
	addresses := []string{testConfig.GrpcFqdn, u.Host}
	for _, address := range addresses {
		host, _, err := net.SplitHostPort(address)
		if err != nil || (host != "localhost" && !net.ParseIP(host).IsLoopback()) {
			t.Fatal("action lifecycle requires loopback endpoints")
		}
		if connection, err := net.DialTimeout("tcp", address, 200*time.Millisecond); err == nil {
			connection.Close()
			t.Fatalf("%s is occupied; the existing server was not touched", address)
		}
	}
	backupMakinoCatalogCollections(t, sname.NoticeCatalog, sname.ShopCatalog, sname.ExchangeCatalog, sname.GachaCatalog, sname.GachaRules)
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	server := startMakinoTestServer(t, binary, repo, filepath.Join(t.TempDir(), "server.log"), addresses)
	t.Cleanup(func() { server.stop(t); testConfig.HttpClient.CloseIdleConnections() })
	for _, scenario := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"WireBodies", TestGRPCActionWireScenario},
		{"DailyRelogin", TestInterceptorDailyRelogin},
		{"Unimplemented", TestInterceptorUnimplementedService},
		{"Idempotency", TestInterceptorAppRequestIdempotency},
		{"Health", TestHealthPingScenario},
		{"Profile", TestProfileCustomizationScenario},
		{"MissingUser", TestProfileMissingUserScenario},
		{"HomePosition", TestHomeSetCharacterPosition},
		{"User", TestUser},
	} {
		t.Run(scenario.name, scenario.run)
	}
}
