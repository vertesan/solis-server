package test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/proto/papi"
)

// Opt-in lifecycle coverage owns its server process and snapshots catalogs
// BEFORE startup, so one-time compatibility initialization is also restored.
func TestMakinoLifecycleScenario(t *testing.T) {
	serverBinary := os.Getenv("MAKINO_TEST_SERVER_BINARY")
	cliBinary := os.Getenv("MAKINO_TEST_CLI_BINARY")
	if serverBinary == "" || cliBinary == "" {
		t.Skip("set MAKINO_TEST_SERVER_BINARY and MAKINO_TEST_CLI_BINARY to run the isolated lifecycle scenario")
	}
	if testConfig.Env != Dev {
		t.Fatal("lifecycle scenario requires the local development test configuration")
	}
	for _, binary := range []string{serverBinary, cliBinary} {
		if !filepath.IsAbs(binary) {
			t.Fatal("lifecycle binary paths must be absolute")
		}
		if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("lifecycle binary is unavailable: %s (%v)", binary, err)
		}
	}
	repoDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	httpURL, err := url.Parse(testConfig.HttpUrl)
	if err != nil || httpURL.Host == "" {
		t.Fatalf("invalid test HTTP URL: %v", err)
	}
	addresses := []string{httpURL.Host, testConfig.GrpcFqdn}
	for _, address := range addresses {
		host, _, err := net.SplitHostPort(address)
		if err != nil || (host != "localhost" && !net.ParseIP(host).IsLoopback()) {
			t.Fatalf("lifecycle refuses non-loopback address %q", address)
		}
		if connection, err := net.DialTimeout("tcp", address, 200*time.Millisecond); err == nil {
			connection.Close()
			t.Fatalf("%s is occupied; stop the existing server before running lifecycle tests (it was not touched)", address)
		}
	}
	logDir := t.TempDir()
	backupMakinoCatalogCollections(t, sname.NoticeCatalog, sname.ShopCatalog, sname.ExchangeCatalog, sname.GachaRules, sname.GachaCatalog)
	var server *makinoTestServerProcess
	t.Cleanup(func() {
		if server != nil {
			server.stop(t)
		}
	})
	server = startMakinoTestServer(t, serverBinary, repoDir, filepath.Join(logDir, "initial.log"), addresses)

	for _, scenario := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"UserAdmin", TestMakinoUserAdminScenario},
		{"UserRequestSafety", TestMakinoUserRequestSafetyScenario},
		{"ServerCommands", TestMakinoServerCommandScenario},
		{"NormalShopEdit", TestMakinoNormalShopEditScenario},
		{"Catalog", TestMakinoCatalogScenario},
		{"GachaRotation", TestMakinoGachaRotationScenario},
		{"GachaValidation", TestMakinoGachaValidationScenario},
		{"Gacha", TestGachaScenario},
		{"GachaProbability", TestGachaProbabilityCatalogScenario},
		{"UserGod", TestUserGod},
	} {
		t.Run(scenario.name, scenario.run)
	}
	if t.Failed() {
		return
	}
	if os.Getenv("MAKINO_TEST_FULL_SUITE") == "1" {
		t.Run("FullRegression", func(t *testing.T) {
			testBinary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			stdoutPath := filepath.Join(logDir, "full-suite.stdout.log")
			stderrPath := filepath.Join(logDir, "full-suite.stderr.log")
			stdout, err := os.Create(stdoutPath)
			if err != nil {
				t.Fatal(err)
			}
			defer stdout.Close()
			stderr, err := os.Create(stderrPath)
			if err != nil {
				t.Fatal(err)
			}
			defer stderr.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 13*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, testBinary, "-test.v", "-test.timeout", "12m")
			cmd.Dir = filepath.Join(repoDir, "test")
			for _, value := range os.Environ() {
				// The child runs ordinary scenarios against our server, not a
				// recursive lifecycle that would compete for ports or backups.
				if !strings.HasPrefix(value, "MAKINO_TEST_SERVER_BINARY=") {
					cmd.Env = append(cmd.Env, value)
				}
			}
			cmd.Stdout, cmd.Stderr = stdout, stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("full regression failed: %v\n%s", err, makinoRegressionSummary(stdoutPath, stderrPath))
			}
			t.Logf("full regression PASS: %s", makinoRegressionSummary(stdoutPath, stderrPath))
		})
		if t.Failed() {
			return
		}
	}
	// Initializers fill only missing documents/legacy fields. A restart must
	// not replay the embedded seeds over intentionally cleared catalogs.
	t.Run("RestartPreservesClear", func(t *testing.T) {
		setupTestUser(t)
		runCLI := func(args ...string) []byte {
			t.Helper()
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, cliBinary, args...)
			cmd.Dir = repoDir
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("catalog command failed: %v\n%s", err, stderr.String())
			}
			return stdout.Bytes()
		}
		runCLI("server", "notice", "--clear")
		runCLI("server", "gacha", "--clear")
		before := make(map[string][]byte)
		for _, kind := range []string{"notice", "shop", "exchange", "gacha", "gacha-rules"} {
			before[kind] = runCLI("server", kind, "--export")
		}
		server.stop(t)
		// The fixture cleanup also uses this shared HTTP client. Its pooled
		// connections belong to the old process and cannot survive the restart.
		testConfig.HttpClient.CloseIdleConnections()
		server = startMakinoTestServer(t, serverBinary, repoDir, filepath.Join(logDir, "restart.log"), addresses)
		for kind, original := range before {
			if after := runCLI("server", kind, "--export"); !bytes.Equal(original, after) {
				t.Fatalf("%s catalog changed during restart", kind)
			}
		}
		gachas := profileCall(t, papi.NewGachaClient(testConfig.GrpcConn).List, &papi.Empty{})
		if len(gachas.GetGachas()) != 0 {
			t.Fatal("restart restored a deliberately cleared gacha roster")
		}
		notices := profileCall(t, papi.NewNoticeClient(testConfig.GrpcConn).List, &papi.Empty{})
		if len(notices.GetNotices())+len(notices.GetMalfunctionNotices())+len(notices.GetPrNotices()) != 0 {
			t.Fatal("restart restored deliberately cleared notices")
		}
		showcase := profileCall(t, papi.NewShowcaseClient(testConfig.GrpcConn).Top, &papi.Empty{})
		if len(showcase.GetShops()) == 0 {
			t.Fatal("no showcase shops available to verify cleared notice")
		}
		for _, shop := range showcase.GetShops() {
			assertDummyNoticeInfo(t, "restart-cleared showcase notice", shop.GetNoticeInfo())
		}
	})
}

// Keep output to test outcomes and short assertion diagnostics. Full protocol
// responses, including authentication logs, stay in the temporary log files.
func makinoRegressionSummary(paths ...string) string {
	passed, failed := 0, 0
	var details, recent []string
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			lower := strings.ToLower(line)
			if strings.Contains(line, "_test.go:") && !strings.Contains(lower, "token") &&
				!strings.Contains(lower, "password") && !strings.Contains(lower, "authorization") {
				if len(line) > 500 {
					line = line[:500] + "..."
				}
				recent = append(recent, line)
				if len(recent) > 3 {
					recent = recent[len(recent)-3:]
				}
			}
			switch {
			case strings.HasPrefix(line, "--- FAIL:"):
				failed++
				if len(details) < 24 {
					details = append(details, line)
					details = append(details, recent...)
				}
				recent = nil
			case strings.HasPrefix(line, "--- PASS:"):
				passed++
				recent = nil
			case strings.HasPrefix(line, "--- SKIP:"):
				recent = nil
			case strings.HasPrefix(line, "panic: test timed out"):
				details = append(details, line)
			}
		}
		file.Close()
	}
	return fmt.Sprintf("%d passed, %d failed\n%s", passed, failed, strings.Join(details, "\n"))
}

type makinoTestServerProcess struct {
	cmd     *exec.Cmd
	done    chan struct{}
	err     error
	logPath string
}

func startMakinoTestServer(t *testing.T, binary, repoDir, logPath string, addresses []string) *makinoTestServerProcess {
	t.Helper()
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary)
	cmd.Dir = repoDir
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		t.Fatal(err)
	}
	process := &makinoTestServerProcess{cmd: cmd, done: make(chan struct{}), logPath: logPath}
	go func() {
		process.err = cmd.Wait()
		logFile.Close()
		close(process.done)
	}()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-process.done:
			t.Fatalf("test server exited before readiness: %v\n%s", process.err, process.logTail())
		case <-deadline.C:
			process.stop(t)
			t.Fatalf("test server did not start within 30 seconds\n%s", process.logTail())
		case <-ticker.C:
			ready := true
			for _, address := range addresses {
				connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
				if err != nil {
					ready = false
					break
				}
				connection.Close()
			}
			if ready {
				return process
			}
		}
	}
}

func (process *makinoTestServerProcess) stop(t *testing.T) {
	t.Helper()
	select {
	case <-process.done:
		return
	default:
	}
	if err := process.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("signal test server: %v", err)
	}
	select {
	case <-process.done:
		if process.err != nil {
			t.Errorf("test server shutdown failed: %v\n%s", process.err, process.logTail())
		}
	case <-time.After(10 * time.Second):
		// Kill only the exact child that this test launched, never a port owner
		// or another process selected by name.
		if err := process.cmd.Process.Kill(); err != nil {
			t.Errorf("kill unresponsive test server: %v", err)
		}
		select {
		case <-process.done:
		case <-time.After(5 * time.Second):
			t.Errorf("test server did not exit after kill")
		}
		t.Errorf("test server required forced termination\n%s", process.logTail())
	}
}

func (process *makinoTestServerProcess) logTail() string {
	data, err := os.ReadFile(process.logPath)
	if err != nil {
		return fmt.Sprintf("cannot read server log: %v", err)
	}
	const maxLogSize = 32 << 10
	if len(data) > maxLogSize {
		data = data[len(data)-maxLogSize:]
	}
	return string(data)
}
