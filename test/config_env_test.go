package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/utils/config"
)

func TestEnvironmentFilePrecedence(t *testing.T) {
	const key = "SOLIS_TEST_ENV_SETTING"
	const emptyKey = "SOLIS_TEST_ENV_EMPTY"
	const retainedKey = "SOLIS_TEST_ENV_RETAINED"
	const expandedKey = "SOLIS_TEST_ENV_EXPANDED"
	t.Setenv(key, "from-compose")
	t.Setenv(emptyKey, "from-compose")
	t.Setenv(retainedKey, "environment-only")
	t.Setenv(expandedKey, "")
	file := filepath.Join(t.TempDir(), ".env")
	contents := key + "=from-file\n" + emptyKey + "=\n" + expandedKey + "=${" + key + "}/resources\n"
	if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.LoadEnvironment(file); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		key:         "from-file",
		emptyKey:    "",
		retainedKey: "environment-only",
		expandedKey: "from-file/resources",
	} {
		if got, exists := os.LookupEnv(name); !exists || got != want {
			t.Errorf("%s: got %q (exists=%v), want %q", name, got, exists, want)
		}
	}
}

func TestEnvironmentFileOptional(t *testing.T) {
	const key = "SOLIS_TEST_ENV_SETTING"
	t.Setenv(key, "environment-only")
	if err := config.LoadEnvironment(filepath.Join(t.TempDir(), ".env")); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(key); got != "environment-only" {
		t.Fatalf("missing file changed process environment: %q", got)
	}
}

func TestEnvironmentFileMalformed(t *testing.T) {
	const key = "SOLIS_TEST_ENV_SETTING"
	const secret = "do-not-print-this-secret"
	t.Setenv(key, "unchanged")
	file := filepath.Join(t.TempDir(), ".env")
	contents := key + "=must-not-apply\nSOLIS_TEST_ENV_BROKEN=\"" + secret + "\n"
	if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	err := config.LoadEnvironment(file)
	if err == nil {
		t.Fatal("malformed dotenv file was accepted")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("dotenv error exposed a secret value")
	}
	if got := os.Getenv(key); got != "unchanged" {
		t.Fatalf("malformed file partially changed process environment: %q", got)
	}
}
