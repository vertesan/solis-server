package test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"google.golang.org/protobuf/proto"
)

func TestMasterExportOutputScenario(t *testing.T) {
	// Only read one small master collection, without a game server or account.
	ctx := context.Background()
	output := masterExportTempOutput(t)
	if err := masterdata.Export(ctx, masterdata.ExportOptions{Type: "ManagerLevel", Output: output}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ManagerLevel.yaml")
	if err := masterdata.Export(ctx, masterdata.ExportOptions{Type: "ManagerLevel", File: path}); err != nil {
		t.Fatal(err)
	}
	assertMasterExportEquivalent(t, masterExportOpen(t, output.Name()), masterExportOpen(t, path))

	rejected := masterExportTempOutput(t)
	if err := masterdata.Export(ctx, masterdata.ExportOptions{Type: "NotAnActiveMasterType", Output: rejected}); err == nil || masterExportSize(t, rejected.Name()) != 0 {
		t.Fatalf("invalid export must fail without writing output: %v", err)
	}
	want := errors.New("output unavailable")
	if err := masterdata.Export(ctx, masterdata.ExportOptions{Type: "ManagerLevel", Output: masterExportFailWriter{want}}); !errors.Is(err, want) {
		t.Fatalf("output write error was not propagated: %v", err)
	}
}

type masterExportFailWriter struct{ err error }

func (writer masterExportFailWriter) Write([]byte) (int, error) { return 0, writer.err }

func masterExportTempOutput(t *testing.T) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "output-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func masterExportOpen(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func masterExportSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func masterExportPrefix(t *testing.T, path string) string {
	t.Helper()
	data, err := io.ReadAll(io.LimitReader(masterExportOpen(t, path), 4096))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertMasterExportEquivalent(t *testing.T, first, second io.Reader) {
	t.Helper()
	left, err := masterdata.NewYAMLReader(first)
	if err != nil {
		t.Fatalf("invalid stdout YAML header: %v", err)
	}
	right, err := masterdata.NewYAMLReader(second)
	if err != nil {
		t.Fatal(err)
	}
	if left.Header != right.Header || left.Header.Type != "ManagerLevel" {
		t.Fatal("stdout and file headers differ or have the wrong type")
	}
	count := 0
	for {
		leftMessage, leftErr := left.Next()
		rightMessage, rightErr := right.Next()
		if errors.Is(leftErr, io.EOF) && errors.Is(rightErr, io.EOF) {
			break
		}
		if leftErr != nil || rightErr != nil {
			t.Fatalf("invalid or mismatched YAML streams at record %d: %v / %v", count, leftErr, rightErr)
		}
		if !proto.Equal(leftMessage, rightMessage) {
			t.Fatalf("stdout and file records differ at index %d", count)
		}
		count++
	}
	if count == 0 {
		t.Fatal("ManagerLevel export unexpectedly empty")
	}
}

func runMasterStdoutCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	binary := os.Getenv("MAKINO_TEST_CLI_BINARY")
	if binary == "" {
		t.Skip("requires MAKINO_TEST_CLI_BINARY")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("MAKINO_TEST_CLI_BINARY must be absolute")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = ".."
	stdout, stderr := masterExportTempOutput(t), masterExportTempOutput(t)
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	return stdout.Name(), masterExportPrefix(t, stderr.Name()), err
}

func TestMasterExportStdoutCLIScenario(t *testing.T) {
	stdout, stderr, err := runMasterStdoutCLI(t, "master", "export", "--type", "ManagerLevel")
	if err != nil {
		t.Fatalf("stdout export failed: %v\n%s", err, stderr)
	}
	if !strings.HasPrefix(masterExportPrefix(t, stdout), "formatVersion:") {
		t.Fatal("stdout contains non-YAML status output")
	}
	path := filepath.Join(t.TempDir(), "ManagerLevel.yaml")
	statusPath, stderr, err := runMasterStdoutCLI(t, "master", "export", "--type", "ManagerLevel", "--file", path)
	status := masterExportPrefix(t, statusPath)
	if err != nil || strings.TrimSpace(status) != "master export complete" || masterExportSize(t, statusPath) != int64(len(status)) {
		t.Fatalf("file export status changed: %v", err)
	}
	assertMasterExportEquivalent(t, masterExportOpen(t, stdout), masterExportOpen(t, path))
	if _, stderr, err := runMasterStdoutCLI(t, "master", "export", "--type", "ManagerLevel", "--file", path); err == nil || !strings.Contains(stderr, "--force") {
		t.Fatalf("file overwrite did not require --force: %v, %s", err, stderr)
	}
	if _, stderr, err := runMasterStdoutCLI(t, "master", "export", "--type", "ManagerLevel", "--file", path, "--force"); err != nil {
		t.Fatalf("forced file export failed: %v, %s", err, stderr)
	}
	for _, args := range [][]string{
		{"master", "export"},
		{"master", "export", "--type", "ManagerLevel", "--all"},
		{"master", "export", "--type", "ManagerLevel", "--file", path, "--dir", filepath.Dir(path)},
		{"master", "import"},
	} {
		stdout, _, err := runMasterStdoutCLI(t, args...)
		if err == nil || masterExportSize(t, stdout) != 0 {
			t.Fatalf("invalid invocation %v did not fail without stdout: %v", args, err)
		}
	}
}

func TestMasterExportAllRequiresDirectory(t *testing.T) {
	t.Run("core", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		output := masterExportTempOutput(t)
		err := masterdata.Export(ctx, masterdata.ExportOptions{All: true, Output: output})
		if err == nil || !strings.Contains(err.Error(), "--dir") || errors.Is(err, context.Canceled) || masterExportSize(t, output.Name()) != 0 {
			t.Fatalf("--all without --dir must be rejected before reading the database: %v", err)
		}
	})
	t.Run("CLI", func(t *testing.T) {
		// An invalid URI proves flag validation happens before a DB connection.
		t.Setenv("SOLIS_DB_URI", "not-a-mongodb-uri")
		stdout, stderr, err := runMasterStdoutCLI(t, "master", "export", "--all")
		if err == nil || !strings.Contains(stderr, "--dir") || masterExportSize(t, stdout) != 0 {
			t.Fatalf("--all without --dir must fail before database access: %v, %s", err, stderr)
		}
	})
}
