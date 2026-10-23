package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Production entry points deliberately let .env override process variables.
// Test children instead inherit the test's configured environment in a fresh
// directory, so loading the repository's .env cannot replace temporary paths.
func isolateTestCommand(t *testing.T, command *exec.Cmd, originalDirectory string) {
	t.Helper()
	directory, err := filepath.Abs(originalDirectory)
	if err != nil {
		t.Fatal(err)
	}
	command.Dir = t.TempDir()
	if command.Env == nil {
		command.Env = os.Environ()
	}
	for index, value := range command.Env {
		key, path, ok := strings.Cut(value, "=")
		if !ok || path == "" || filepath.IsAbs(path) {
			continue
		}
		switch key {
		case "SOLIS_RESOURCE_DIR", "SOLIS_JWT_RSA_PRIV_KEY_PATH", "SOLIS_JWT_RSA_PUB_KEY_PATH",
			"SOLIS_GRPC_CERT_PRIV_KEY_PATH", "SOLIS_GRPC_CERT_PUB_KEY_PATH":
			command.Env[index] = key + "=" + filepath.Join(directory, path)
		}
	}
}

func TestIsolatedCommandEnvironment(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, ".env"), []byte("SOLIS_RESOURCE_DIR=must-not-use\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("not-executed")
	command.Env = []string{"SOLIS_RESOURCE_DIR=fixture-resources", "SOLIS_JWT_RSA_PRIV_KEY_PATH=keys/private.pem", "SOLIS_ADMIN_AUTH="}
	isolateTestCommand(t, command, directory)
	if command.Dir == directory {
		t.Fatal("subprocess still uses the directory containing .env")
	}
	if _, err := os.Stat(filepath.Join(command.Dir, ".env")); !os.IsNotExist(err) {
		t.Fatalf("isolated command can load an unexpected dotenv file: %v", err)
	}
	want := []string{
		"SOLIS_RESOURCE_DIR=" + filepath.Join(directory, "fixture-resources"),
		"SOLIS_JWT_RSA_PRIV_KEY_PATH=" + filepath.Join(directory, "keys/private.pem"),
		"SOLIS_ADMIN_AUTH=",
	}
	for index, value := range want {
		if command.Env[index] != value {
			t.Errorf("environment entry %d did not preserve its configured value", index)
		}
	}
}
