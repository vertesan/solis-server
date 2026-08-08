package crypto

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/db/model/master/mdoc"
	mversion "github.com/vertesan/solis-server/internal/db/store/opmaster/version"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestDecryptToSliceReadsAllRowsAndBuildsDeterministicNaturalKey verifies full
// row conversion, byte preservation, and stable sorted natural-key metadata.
func TestDecryptToSliceReadsAllRowsAndBuildsDeterministicNaturalKey(t *testing.T) {
	const key = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	dbPath := filepath.Join(t.TempDir(), "master.db")
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_pragma_key=x'%s'", dbPath, key))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE "Example" ("zeta" TEXT NOT NULL, "data" BLOB NOT NULL, "alpha" INTEGER NOT NULL)`); err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO "Example" ("zeta", "data", "alpha") VALUES (?, ?, ?), (?, ?, ?)`,
		"second", []byte{2}, 2,
		"first", []byte{1}, 1,
	); err != nil {
		t.Fatalf("failed to insert test rows: %v", err)
	}

	documents, indexKeys, err := decryptToSlice(context.Background(), "Example", dbPath, key)
	if err != nil {
		t.Fatalf("decryptToSlice returned an error: %v", err)
	}
	if len(documents) != 2 {
		t.Fatalf("unexpected document count: got %d, want 2", len(documents))
	}
	wantKeys := bson.D{{Key: "alpha", Value: 1}, {Key: "zeta", Value: 1}}
	if !reflect.DeepEqual(indexKeys, wantKeys) {
		t.Fatalf("unexpected index keys: got %#v, want %#v", indexKeys, wantKeys)
	}
	if got, ok := documents[0]["data"].([]byte); !ok || !reflect.DeepEqual(got, []byte{2}) {
		t.Fatalf("data column was not preserved as bytes: %#v", documents[0]["data"])
	}
	if got, want := documents[0][mdoc.MasterNaturalKeyField], mversion.MasterNaturalKeyMarker(wantKeys); got != want {
		t.Fatalf("unexpected natural-key marker: got %v, want %v", got, want)
	}
}

// TestDecryptToSliceRejectsTableWithoutDataColumn verifies every imported table
// must provide the protobuf payload expected by master queries.
func TestDecryptToSliceRejectsTableWithoutDataColumn(t *testing.T) {
	const key = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	dbPath := filepath.Join(t.TempDir(), "master.db")
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_pragma_key=x'%s'", dbPath, key))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE "Invalid" ("id" INTEGER NOT NULL)`); err != nil {
		_ = db.Close()
		t.Fatalf("failed to create test table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("failed to close test database: %v", err)
	}

	_, _, err = decryptToSlice(context.Background(), "Invalid", dbPath, key)
	if err == nil || !strings.Contains(err.Error(), "has no data column") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDecryptToSliceRejectsDuplicateNaturalKeys verifies conflicting source
// rows are rejected before the replacement transaction starts.
func TestDecryptToSliceRejectsDuplicateNaturalKeys(t *testing.T) {
	const key = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	dbPath := filepath.Join(t.TempDir(), "master.db")
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_pragma_key=x'%s'", dbPath, key))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE "Duplicate" ("id" INTEGER NOT NULL, "data" BLOB NOT NULL)`); err != nil {
		_ = db.Close()
		t.Fatalf("failed to create test table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO "Duplicate" ("id", "data") VALUES (?, ?), (?, ?)`, 1, []byte{1}, 1, []byte{2}); err != nil {
		_ = db.Close()
		t.Fatalf("failed to insert test rows: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("failed to close test database: %v", err)
	}

	_, _, err = decryptToSlice(context.Background(), "Duplicate", dbPath, key)
	if err == nil || !strings.Contains(err.Error(), "duplicate natural key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDecryptToSliceRejectsNonBinaryData verifies master protobuf payloads stay
// binary during SQLite-to-MongoDB conversion.
func TestDecryptToSliceRejectsNonBinaryData(t *testing.T) {
	const key = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	dbPath := filepath.Join(t.TempDir(), "master.db")
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_pragma_key=x'%s'", dbPath, key))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE "InvalidData" ("id" INTEGER NOT NULL, "data" TEXT NOT NULL)`); err != nil {
		_ = db.Close()
		t.Fatalf("failed to create test table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO "InvalidData" ("id", "data") VALUES (?, ?)`, 1, "not protobuf bytes"); err != nil {
		_ = db.Close()
		t.Fatalf("failed to insert test row: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("failed to close test database: %v", err)
	}

	_, _, err = decryptToSlice(context.Background(), "InvalidData", dbPath, key)
	if err == nil || !strings.Contains(err.Error(), "non-binary data column") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDecryptToSliceRejectsReservedNaturalKeyMarkerColumn verifies source data
// cannot overwrite the importer-managed uniqueness marker.
func TestDecryptToSliceRejectsReservedNaturalKeyMarkerColumn(t *testing.T) {
	const key = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	dbPath := filepath.Join(t.TempDir(), "master.db")
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_pragma_key=x'%s'", dbPath, key))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE "Reserved" ("id" INTEGER NOT NULL, "data" BLOB NOT NULL, "_masterNaturalKey" TEXT NOT NULL)`); err != nil {
		_ = db.Close()
		t.Fatalf("failed to create test table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("failed to close test database: %v", err)
	}

	_, _, err = decryptToSlice(context.Background(), "Reserved", dbPath, key)
	if err == nil || !strings.Contains(err.Error(), "uses reserved column") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestMasterCollectionsIncludesObsoleteTypesInDeterministicOrder verifies stale
// collections are included for deletion and the result order is stable.
func TestMasterCollectionsIncludesObsoleteTypesInDeterministicOrder(t *testing.T) {
	existing := []string{"Stale", "Current", "", "_version", "system.views"}
	prepared := []preparedMaster{{name: "Added"}, {name: "Current"}}

	got := masterCollections(existing, prepared)
	want := []string{"Added", "Current", "Stale"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected collections: got %v, want %v", got, want)
	}
}

// TestPrepareAllMasterRejectsDuplicatePackTypesBeforeReadingFiles verifies
// ambiguous releases fail during metadata validation, before file access.
func TestPrepareAllMasterRejectsDuplicatePackTypesBeforeReadingFiles(t *testing.T) {
	masterTag := &ptransaction.MasterTag{
		Version: "target",
		MasterTagPacks: []*ptransaction.MasterTagPack{
			{Type: "Duplicate", FileName: "first", CryptoKey: "key"},
			{Type: "Duplicate", FileName: "second", CryptoKey: "key"},
		},
	}

	_, err := prepareAllMaster(context.Background(), masterTag)
	if err == nil || !strings.Contains(err.Error(), `duplicate master tag pack type "Duplicate"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
