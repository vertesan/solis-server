package update

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// TestSameMasterTagPackTypes covers exact-set matching and the legacy states
// that must trigger a full master release repair.
func TestSameMasterTagPackTypes(t *testing.T) {
	pack := func(name string) *ptransaction.MasterTagPack {
		return &ptransaction.MasterTagPack{Type: name}
	}
	tests := []struct {
		name   string
		stored []string
		target []*ptransaction.MasterTagPack
		want   bool
	}{
		{
			name:   "same unordered set",
			stored: []string{"B", "A"},
			target: []*ptransaction.MasterTagPack{pack("A"), pack("B")},
			want:   true,
		},
		{
			name:   "legacy stale type",
			stored: []string{"A", "Removed"},
			target: []*ptransaction.MasterTagPack{pack("A"), pack("B")},
		},
		{
			name:   "missing target type",
			stored: []string{"A"},
			target: []*ptransaction.MasterTagPack{pack("A"), pack("B")},
		},
		{
			name:   "duplicate target type",
			stored: []string{"A", "A"},
			target: []*ptransaction.MasterTagPack{pack("A"), pack("A")},
		},
		{
			name:   "empty stored type",
			stored: []string{""},
			target: []*ptransaction.MasterTagPack{pack("A")},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sameMasterTagPackTypes(test.stored, test.target); got != test.want {
				t.Fatalf("sameMasterTagPackTypes() = %t, want %t", got, test.want)
			}
		})
	}
}

// TestShouldReplaceOctoManifest verifies idempotence, stale-writer detection,
// and downgrade prevention for manifest replacement.
func TestShouldReplaceOctoManifest(t *testing.T) {
	tests := []struct {
		name     string
		observed int32
		target   int32
		current  int32
		want     bool
		wantErr  bool
	}{
		{name: "unchanged observed revision", observed: 1, target: 2, current: 1, want: true},
		{name: "target committed concurrently", observed: 1, target: 2, current: 2},
		{name: "different revision committed concurrently", observed: 1, target: 2, current: 3, wantErr: true},
		{name: "stale target revision", observed: 3, target: 2, current: 3, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := shouldReplaceOctoManifest(test.observed, test.target, test.current)
			if (err != nil) != test.wantErr {
				t.Fatalf("shouldReplaceOctoManifest() error = %v, wantErr %t", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("shouldReplaceOctoManifest() = %t, want %t", got, test.want)
			}
		})
	}
}

// TestValidateMasterTagPacksForDownloadRejectsDuplicateFileNames verifies two
// concurrent pack downloads cannot target the same final file.
func TestValidateMasterTagPacksForDownloadRejectsDuplicateFileNames(t *testing.T) {
	masterTag := &ptransaction.MasterTag{Version: "target", MasterTagPacks: []*ptransaction.MasterTagPack{
		{Type: "A", FileName: "same", FileSize: 1, DownloadUrl: "https://example.test/a", CryptoKey: "key"},
		{Type: "B", FileName: "same", FileSize: 1, DownloadUrl: "https://example.test/b", CryptoKey: "key"},
	}}
	if err := validateMasterTagPacksForDownload(masterTag); err == nil || !strings.Contains(err.Error(), "duplicate master tag pack file name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestPublishMasterTagPackPublishesCompleteFile verifies a complete temporary
// download is atomically published with unchanged contents.
func TestPublishMasterTagPackPublishesCompleteFile(t *testing.T) {
	payload := []byte("encrypted master database")
	filePath := filepath.Join(t.TempDir(), "master.db")
	pack := &ptransaction.MasterTagPack{FileSize: int32(len(payload))}

	if err := publishMasterTagPack(filePath, pack, bytes.NewReader(payload)); err != nil {
		t.Fatalf("publishMasterTagPack returned an error: %v", err)
	}
	got, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read published file: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("unexpected published payload: got %q, want %q", got, payload)
	}
	assertNoDownloadTempFiles(t, filePath)
}

// TestPublishMasterTagPackPreservesFinalFileOnShortDownload verifies an invalid
// replacement leaves the previous complete cache file intact.
func TestPublishMasterTagPackPreservesFinalFileOnShortDownload(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "master.db")
	original := []byte("previous complete file")
	if err := os.WriteFile(filePath, original, 0o644); err != nil {
		t.Fatalf("failed to write existing file: %v", err)
	}
	pack := &ptransaction.MasterTagPack{FileSize: 100}

	err := publishMasterTagPack(filePath, pack, strings.NewReader("short"))
	if err == nil || !strings.Contains(err.Error(), "unexpected master database size") {
		t.Fatalf("unexpected error: %v", err)
	}
	got, readErr := os.ReadFile(filePath)
	if readErr != nil {
		t.Fatalf("failed to read existing file: %v", readErr)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("existing file changed after failed download: got %q, want %q", got, original)
	}
	assertNoDownloadTempFiles(t, filePath)
}

// assertNoDownloadTempFiles fails a test when publishMasterTagPack leaves a
// temporary file beside the target path.
func assertNoDownloadTempFiles(t *testing.T, filePath string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(filePath), "."+filepath.Base(filePath)+".tmp-*"))
	if err != nil {
		t.Fatalf("failed to inspect temporary files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary download files remain: %v", matches)
	}
}
