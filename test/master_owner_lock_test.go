package test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/utils/config"
)

func TestMasterOwnershipScenario(t *testing.T) {
	t.Setenv("SOLIS_RESOURCE_DIR", t.TempDir())
	owner, err := masterdata.AcquireOwnership()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	name := filepath.Join(config.GetDBDir(), ".master-owner.lock")
	before, err := os.Lstat(name)
	if err != nil || !before.Mode().IsRegular() {
		t.Fatalf("ownership did not create a regular lock file: %v", err)
	}
	if second, err := masterdata.AcquireOwnership(); !errors.Is(err, masterdata.ErrOwnershipBusy) {
		if second != nil {
			_ = second.Close()
		}
		t.Fatalf("concurrent owner was not rejected: %v", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("repeated close failed: %v", err)
	}
	after, err := os.Lstat(name)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("ownership release replaced or removed its lock file: %v", err)
	}
	owner, err = masterdata.AcquireOwnership()
	if err != nil {
		t.Fatalf("released ownership could not be reacquired: %v", err)
	}
}

func TestMasterOwnershipProcessExit(t *testing.T) {
	t.Setenv("SOLIS_RESOURCE_DIR", t.TempDir())
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-test.run=^TestMasterOwnershipProcessHelper$", "-test.timeout=25s")
	command.Env = append(os.Environ(), "SOLIS_TEST_MASTER_OWNERSHIP_HELPER=1")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "master ownership ready\n" {
		_ = command.Process.Kill()
		_ = command.Wait()
		waited = true
		t.Fatalf("ownership subprocess did not start: %q, %v, %s", line, err, stderr.String())
	}
	if owner, err := masterdata.AcquireOwnership(); !errors.Is(err, masterdata.ErrOwnershipBusy) {
		if owner != nil {
			_ = owner.Close()
		}
		t.Fatalf("cross-process owner was not rejected: %v", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	waited = true
	owner, err := masterdata.AcquireOwnership()
	if err != nil {
		t.Fatalf("process exit left ownership locked: %v", err)
	}
	t.Cleanup(func() { _ = owner.Close() })
}

func TestMasterOwnershipProcessHelper(t *testing.T) {
	if os.Getenv("SOLIS_TEST_MASTER_OWNERSHIP_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	owner, err := masterdata.AcquireOwnership()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	fmt.Println("master ownership ready")
	// The parent kills this process without allowing the deferred Close to run.
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestMasterOwnershipPathValidation(t *testing.T) {
	t.Run("nonregular lock", func(t *testing.T) {
		t.Setenv("SOLIS_RESOURCE_DIR", t.TempDir())
		if err := os.MkdirAll(filepath.Join(config.GetDBDir(), ".master-owner.lock"), 0755); err != nil {
			t.Fatal(err)
		}
		assertMasterOwnershipPathRejected(t)
	})
	t.Run("symlink lock", func(t *testing.T) {
		t.Setenv("SOLIS_RESOURCE_DIR", t.TempDir())
		if err := os.MkdirAll(config.GetDBDir(), 0755); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "target")
		if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(config.GetDBDir(), ".master-owner.lock")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		assertMasterOwnershipPathRejected(t)
		content, err := os.ReadFile(target)
		if err != nil || string(content) != "untouched" {
			t.Fatalf("lock acquisition changed symlink target: %q, %v", content, err)
		}
	})
	t.Run("symlink resource database directory", func(t *testing.T) {
		t.Setenv("SOLIS_RESOURCE_DIR", t.TempDir())
		if err := os.Symlink(t.TempDir(), config.GetDBDir()); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		assertMasterOwnershipPathRejected(t)
	})
}

func assertMasterOwnershipPathRejected(t *testing.T) {
	t.Helper()
	owner, err := masterdata.AcquireOwnership()
	if owner != nil {
		_ = owner.Close()
	}
	if err == nil {
		t.Fatal("unsafe ownership lock path was accepted")
	}
}
