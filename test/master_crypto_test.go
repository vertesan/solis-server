package test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/crypto"
	"google.golang.org/protobuf/proto"
)

func TestMasterCryptoFreshDatabaseScenario(t *testing.T) {
	ctx := context.Background()
	key := strings.Repeat("ab", 32)
	directory := t.TempDir()
	source := filepath.Join(directory, "source.sqlite")
	uri := url.URL{Scheme: "file", Path: source}
	uri.RawQuery = url.Values{"_pragma_key": {"x'" + key + "'"}}.Encode()
	db, err := sql.Open("sqlite3", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`PRAGMA user_version = 73`,
		`PRAGMA application_id = 2147483647`,
		`CREATE TABLE Card (id TEXT NOT NULL PRIMARY KEY, initialRarity INTEGER NOT NULL CHECK(initialRarity >= 1), data BLOB NOT NULL)`,
		`CREATE INDEX rarity_index ON Card(initialRarity)`,
		`CREATE TABLE metadata (name TEXT PRIMARY KEY, value BLOB, big INTEGER)`,
		`INSERT INTO metadata VALUES ('format', x'010200ff', 9223372036854775807)`,
		`CREATE VIEW card_ids AS SELECT id FROM Card`,
		`CREATE TRIGGER check_card AFTER INSERT ON Card BEGIN SELECT 1; END`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	oldBlob, err := proto.Marshal(&pmaster.Card{Id: "old-only", Name: "Old", InitialRarity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO Card VALUES (?, ?, ?)`, "old-only", 1, oldBlob); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	newBlob, err := proto.Marshal(&pmaster.Card{Id: "custom-card", Name: "Custom", InitialRarity: 5})
	if err != nil {
		t.Fatal(err)
	}
	newRow := map[string]any{"id": "custom-card", "initialRarity": int64(5), "data": newBlob}
	destination := filepath.Join(directory, "fresh.sqlite")
	if err := crypto.BuildMasterDb(ctx, source, destination, key, "Card", func(yield func(map[string]any) error) error {
		return yield(newRow)
	}); err != nil {
		t.Fatal(err)
	}
	schema, err := crypto.InspectMasterDb(ctx, destination, key, "Card")
	if err != nil {
		t.Fatal(err)
	}
	if schema.RowCount != 1 || schema.Settings["user_version"] != "73" || len(schema.Columns) != 3 || schema.Columns[0].PrimaryKey != 1 {
		t.Fatalf("fresh schema lost compatibility metadata: %+v", schema)
	}
	rows, err := crypto.DecryptMasterDbFile("Card", destination, key)
	if err != nil || len(rows) != 1 || !reflect.DeepEqual(rows[0], newRow) {
		t.Fatalf("fresh encrypted rows differ: rows=%d err=%v", len(rows), err)
	}
	var decoded pmaster.Card
	if err := proto.Unmarshal(rows[0]["data"].([]byte), &decoded); err != nil || decoded.GetName() != "Custom" {
		t.Fatalf("fresh protobuf round trip failed: %v", err)
	}
	if err := crypto.IterateMasterDb(ctx, destination, key, "metadata", func(row map[string]any) error {
		if !bytes.Equal(row["value"].([]byte), []byte{1, 2, 0, 255}) || row["big"] != int64(math.MaxInt64) {
			t.Fatal("auxiliary metadata lost binary or integer precision")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	oldBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	newBytes, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(oldBytes[:16], newBytes[:16]) || bytes.HasPrefix(newBytes, []byte("SQLite format 3")) {
		t.Fatal("fresh encrypted database did not receive a fresh salt")
	}
	if err := crypto.BuildMasterDb(ctx, source, destination, key, "Card", func(yield func(map[string]any) error) error {
		return yield(newRow)
	}); err == nil {
		t.Fatal("existing encrypted file must never be overwritten")
	}
	for _, scenario := range []string{"duplicate", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(directory, scenario)
			err := crypto.BuildMasterDb(ctx, source, path, key, "Card", func(yield func(map[string]any) error) error {
				if err := yield(newRow); err != nil {
					return err
				}
				if scenario == "duplicate" {
					return yield(newRow)
				}
				return context.Canceled
			})
			if err == nil {
				t.Fatal("invalid build unexpectedly succeeded")
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed build left an incomplete master file")
			}
		})
	}
	missing := filepath.Join(directory, "missing")
	if _, err := crypto.DecryptMasterDbFile("Card", missing, key); err == nil {
		t.Fatal("missing database must not be created while decrypting")
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read-only decrypt created an empty file")
	}
	if _, err := crypto.DecryptMasterDbFile(`Card; DROP TABLE Card; --`, source, key); err == nil {
		t.Fatal("malformed identifier should not select or modify the master table")
	}
	if err := crypto.ValidateMasterDb(ctx, source, destination, key, "Card"); err != nil {
		t.Fatal(err)
	}
	// Publication must distrust staged files, even when they decrypt correctly.
	uri.Path = destination
	tampered, err := sql.Open("sqlite3", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tampered.ExecContext(ctx, `UPDATE metadata SET big = 7`); err != nil {
		tampered.Close()
		t.Fatal(err)
	}
	if err := tampered.Close(); err != nil {
		t.Fatal(err)
	}
	if err := crypto.ValidateMasterDb(ctx, source, destination, key, "Card"); err == nil {
		t.Fatal("publication accepted a staged database with altered auxiliary metadata")
	}
	tampered, err = sql.Open("sqlite3", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tampered.ExecContext(ctx, `UPDATE metadata SET big = 9223372036854775807; CREATE INDEX unexpected_index ON Card(id)`); err != nil {
		tampered.Close()
		t.Fatal(err)
	}
	if err := tampered.Close(); err != nil {
		t.Fatal(err)
	}
	if err := crypto.ValidateMasterDb(ctx, source, destination, key, "Card"); err == nil {
		t.Fatal("publication accepted a staged database with altered schema")
	}
	empty := filepath.Join(directory, "empty.sqlite")
	if err := crypto.BuildMasterDb(ctx, source, empty, key, "Card", func(func(map[string]any) error) error { return nil }); err != nil {
		t.Fatal(err)
	}
	rows, err = crypto.DecryptMasterDbFile("Card", empty, key)
	if err != nil || len(rows) != 0 {
		t.Fatal("fresh empty database unexpectedly retained source master rows")
	}
	symlink := filepath.Join(directory, "symlink.sqlite")
	if err := os.Symlink(source, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := crypto.DecryptMasterDbFile("Card", symlink, key); err == nil {
		t.Fatal("decrypt accepted a non-regular symbolic-link source")
	}
}

func TestMasterCryptoOfficialPacksScenario(t *testing.T) {
	ctx := context.Background()
	manifestBytes, err := os.ReadFile(filepath.Join(config.GetManifestDir(), "mastertag.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest ptransaction.MasterTag
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	all := os.Getenv("SOLIS_TEST_MASTER_ALL_PACKS") == "1"
	directory := t.TempDir()
	for _, pack := range manifest.GetMasterTagPacks() {
		if !all && pack.Type != "Card" && pack.Type != "LoveLevel" && pack.Type != "QuestAudienceAdvantage" {
			continue
		}
		t.Run(pack.Type, func(t *testing.T) {
			source := filepath.Join(config.GetDBDir(), pack.FileName)
			schema, err := crypto.InspectMasterDb(ctx, source, pack.CryptoKey, pack.Type)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("rows=%d columns=%+v objects=%+v", schema.RowCount, schema.Columns, schema.Objects)
			if err := crypto.BuildMasterDb(ctx, source, filepath.Join(directory, pack.Type), pack.CryptoKey, pack.Type, func(yield func(map[string]any) error) error {
				return crypto.IterateMasterDb(ctx, source, pack.CryptoKey, pack.Type, yield)
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMasterCryptoAffinityScenario(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, columnType := range []string{"STRING", "INTEGER", "TEXT", "BLOB", "REAL"} {
		t.Run(columnType, func(t *testing.T) {
			if _, err := db.Exec(`DROP TABLE IF EXISTS scalar`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`CREATE TABLE scalar (value ` + columnType + `)`); err != nil {
				t.Fatal(err)
			}
			for _, input := range []any{
				"-001", "-1", "+002", "  +003\t", "", "card-001", "0x12", "3.0e+5", "0.00000000000000001", "-0.0",
				"9223372036854775807", "-9223372036854775808", "9223372036854775808", "-9223372036854775809",
				"9007199254740993", "-9.223372036854776e18", "1e999", "1e-999", "12.3junk", ".5", "3.",
				int64(math.MaxInt64), int64(math.MinInt64), int64(9007199254740993), float64(0.1), float64(3), float64(math.MinInt64),
				[]byte{0, 1, 255}, []byte{}, nil,
			} {
				if _, err := db.Exec(`DELETE FROM scalar`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO scalar(value) VALUES (?)`, input); err != nil {
					t.Fatal(err)
				}
				var actual any
				if err := db.QueryRow(`SELECT value FROM scalar`).Scan(&actual); err != nil {
					t.Fatal(err)
				}
				expected, err := crypto.NormalizeMasterValue(crypto.MasterDbColumn{Name: "value", Type: columnType}, input)
				if err != nil || !reflect.DeepEqual(actual, expected) {
					t.Fatalf("affinity %s for %v (%T): expected SQLite %v (%T), got %v (%T), err=%v", columnType, input, input, actual, actual, expected, expected, err)
				}
			}
		})
	}
	// Changing how a numeric-looking string is written must not evade the
	// original SQL primary-key constraint, and integer boundaries stay exact.
	key := strings.Repeat("cd", 32)
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	uri := url.URL{Scheme: "file", Path: source}
	uri.RawQuery = url.Values{"_pragma_key": {"x'" + key + "'"}}.Encode()
	encrypted, err := sql.Open("sqlite3", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encrypted.Exec(`CREATE TABLE GachaExchange(Id STRING PRIMARY KEY, data BLOB)`); err != nil {
		encrypted.Close()
		t.Fatal(err)
	}
	if err := encrypted.Close(); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(directory, "result")
	ids := []string{"-001", "9007199254740993", strconv.FormatInt(math.MaxInt64, 10), strconv.FormatInt(math.MinInt64, 10)}
	if err := crypto.BuildMasterDb(context.Background(), source, destination, key, "GachaExchange", func(yield func(map[string]any) error) error {
		for _, id := range ids {
			data, err := proto.Marshal(&pmaster.GachaExchange{Id: id})
			if err != nil {
				return err
			}
			if err := yield(map[string]any{"Id": id, "data": data}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := crypto.DecryptMasterDbFile("GachaExchange", destination, key)
	if err != nil || len(rows) != len(ids) {
		t.Fatalf("numeric ID round trip failed: %v", err)
	}
	for i, row := range rows {
		var message pmaster.GachaExchange
		if err := proto.Unmarshal(row["data"].([]byte), &message); err != nil || message.Id != ids[i] {
			t.Fatal("normalization altered protobuf string content")
		}
		integer, _ := strconv.ParseInt(ids[i], 10, 64)
		if row["Id"] != integer {
			t.Fatal("indexed integer lost precision")
		}
	}
	if err := crypto.BuildMasterDb(context.Background(), source, filepath.Join(directory, "duplicate"), key, "GachaExchange", func(yield func(map[string]any) error) error {
		for _, id := range []string{"-001", "-1"} {
			if err := yield(map[string]any{"Id": id, "data": []byte{10, 2, 45, 49}}); err != nil {
				return err
			}
		}
		return nil
	}); err == nil {
		t.Fatal("numeric-affinity duplicate keys were accepted")
	}
}
