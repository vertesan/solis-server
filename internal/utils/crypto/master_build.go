package crypto

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"fmt"
	"hash"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// BuildMasterDb creates a fresh encrypted SQLite file, retaining only trusted
// schema/settings and auxiliary metadata from sourcePath. The selected master
// table is populated exclusively by produce, which is called exactly once.
// destPath must not already exist; a failed build removes its own incomplete file.
func BuildMasterDb(ctx context.Context, sourcePath, destPath, key, table string, produce func(func(map[string]any) error) error) (resultErr error) {
	if produce == nil {
		return fmt.Errorf("master row producer is required")
	}
	source, err := openMasterDb(sourcePath, key, true)
	if err != nil {
		return err
	}
	defer source.Close()
	schema, err := inspectMasterDb(ctx, source, table)
	if err != nil {
		return err
	}
	// Never truncate a previous pack, staging artifact, or symbolic link.
	file, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("create new encrypted master file: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(destPath)
		return err
	}
	defer func() {
		if resultErr != nil {
			os.Remove(destPath)
		}
	}()
	destination, err := openMasterDb(destPath, key, false)
	if err != nil {
		return err
	}
	defer destination.Close()
	if err := applyMasterSettings(ctx, destination, schema.Settings); err != nil {
		return err
	}
	tx, err := destination.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin encrypted master creation: %w", err)
	}
	defer tx.Rollback()
	for _, object := range schema.Objects {
		if object.Type != "table" || object.Name == "sqlite_sequence" {
			continue
		}
		if _, err := tx.ExecContext(ctx, object.SQL); err != nil {
			return fmt.Errorf("create master schema table %q: %w", object.Name, err)
		}
	}
	insert, err := masterInsertSQL(table, schema.Columns)
	if err != nil {
		return err
	}
	statement, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("prepare master row insertion: %w", err)
	}
	var expected masterRowDigest
	err = produce(func(row map[string]any) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		values, err := masterRowValues(row, schema.Columns)
		if err != nil {
			return err
		}
		if _, err := statement.ExecContext(ctx, values...); err != nil {
			return fmt.Errorf("insert customized master row %d: %w", expected.Count+1, err)
		}
		return expected.add(values)
	})
	statement.Close()
	if err != nil {
		return err
	}
	// Copy metadata only, not the active master table's records. Triggers and
	// explicit indexes are created afterwards to avoid replaying side effects.
	for _, object := range schema.Objects {
		if object.Type != "table" || object.Name == table {
			continue
		}
		quoted, err := quoteMasterIdentifier(object.Name)
		if err != nil {
			return err
		}
		columns, err := masterColumns(ctx, source, quoted)
		if err != nil {
			return err
		}
		for _, column := range columns {
			if column.Hidden != 0 {
				return fmt.Errorf("generated auxiliary columns are unsupported: %q", object.Name)
			}
		}
		if object.Name == "sqlite_sequence" {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+quoted); err != nil {
				return err
			}
		}
		insert, err := masterInsertSQL(object.Name, columns)
		if err != nil {
			return err
		}
		statement, err := tx.PrepareContext(ctx, insert)
		if err != nil {
			return err
		}
		err = iterateMasterRows(ctx, source, object.Name, func(row map[string]any) error {
			values, err := masterRowValues(row, columns)
			if err != nil {
				return err
			}
			_, err = statement.ExecContext(ctx, values...)
			return err
		})
		statement.Close()
		if err != nil {
			return fmt.Errorf("copy auxiliary table %q: %w", object.Name, err)
		}
	}
	for _, object := range schema.Objects {
		if object.Type == "table" || object.SQL == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, object.SQL); err != nil {
			return fmt.Errorf("create master schema %s %q: %w", object.Type, object.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit encrypted master creation: %w", err)
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("close encrypted master: %w", err)
	}
	if err := ValidateMasterDb(ctx, sourcePath, destPath, key, table); err != nil {
		return err
	}
	var actual masterRowDigest
	if err := IterateMasterDb(ctx, destPath, key, table, func(row map[string]any) error {
		values, err := masterRowValues(row, schema.Columns)
		if err != nil {
			return err
		}
		return actual.add(values)
	}); err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("reopened encrypted master rows differ from customized input")
	}
	file, err = os.OpenFile(destPath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func applyMasterSettings(ctx context.Context, db *sql.DB, settings map[string]string) error {
	if settings["cipher_plaintext_header_size"] != "0" {
		return fmt.Errorf("SQLCipher plaintext headers requiring external salts are unsupported")
	}
	for _, name := range []string{
		"cipher_page_size", "cipher_use_hmac", "cipher_hmac_algorithm", "cipher_kdf_algorithm", "kdf_iter", "cipher_plaintext_header_size",
		"encoding", "auto_vacuum", "user_version", "application_id",
	} {
		value, exists := settings[name]
		if !exists {
			return fmt.Errorf("missing trusted SQLite setting %s", name)
		}
		// Names are constants; quote values even though they came from trusted
		// PRAGMAs, never from editable input.
		if _, err := db.ExecContext(ctx, "PRAGMA "+name+" = '"+strings.ReplaceAll(value, "'", "''")+"'"); err != nil {
			return fmt.Errorf("apply SQLite setting %s: %w", name, err)
		}
	}
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode = DELETE"); err != nil {
		return fmt.Errorf("disable master WAL journal: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA synchronous = FULL"); err != nil {
		return err
	}
	return nil
}

// ValidateMasterDb checks an untrusted staged pack against the active pack's
// trusted schema/settings and auxiliary tables. Selected master rows may differ;
// callers must additionally validate their protobuf payloads and indexed fields.
func ValidateMasterDb(ctx context.Context, sourcePath, candidatePath, key, table string) error {
	source, err := openMasterDb(sourcePath, key, true)
	if err != nil {
		return err
	}
	defer source.Close()
	candidate, err := openMasterDb(candidatePath, key, true)
	if err != nil {
		return err
	}
	defer candidate.Close()
	sourceSchema, err := inspectMasterDb(ctx, source, table)
	if err != nil {
		return err
	}
	candidateSchema, err := inspectMasterDb(ctx, candidate, table)
	if err != nil {
		return fmt.Errorf("inspect generated master database: %w", err)
	}
	if !reflect.DeepEqual(sourceSchema.Objects, candidateSchema.Objects) ||
		!reflect.DeepEqual(sourceSchema.Columns, candidateSchema.Columns) ||
		!reflect.DeepEqual(sourceSchema.Settings, candidateSchema.Settings) {
		return fmt.Errorf("generated master database changed trusted schema or compatibility settings")
	}
	var journalMode string
	if err := candidate.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return err
	}
	if journalMode != "delete" {
		return fmt.Errorf("generated master database must use a self-contained DELETE journal")
	}
	for _, pragma := range []string{"integrity_check", "cipher_integrity_check"} {
		rows, err := candidate.QueryContext(ctx, "PRAGMA "+pragma)
		if err != nil {
			return fmt.Errorf("validate generated master %s: %w", pragma, err)
		}
		for rows.Next() {
			var message string
			if err := rows.Scan(&message); err != nil {
				rows.Close()
				return err
			}
			if message != "ok" {
				rows.Close()
				return fmt.Errorf("generated master database failed %s", pragma)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	rows, err := candidate.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	invalidForeignKey := rows.Next()
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if invalidForeignKey {
		return fmt.Errorf("generated master database violates foreign key constraints")
	}
	for _, object := range sourceSchema.Objects {
		if object.Type != "table" || object.Name == table {
			continue
		}
		quoted, err := quoteMasterIdentifier(object.Name)
		if err != nil {
			return err
		}
		columns, err := masterColumns(ctx, source, quoted)
		if err != nil {
			return err
		}
		var expected, actual masterRowDigest
		for _, item := range []struct {
			db     *sql.DB
			digest *masterRowDigest
		}{{source, &expected}, {candidate, &actual}} {
			if err := iterateMasterRows(ctx, item.db, object.Name, func(row map[string]any) error {
				values, err := masterRowValues(row, columns)
				if err != nil {
					return err
				}
				return item.digest.add(values)
			}); err != nil {
				return err
			}
		}
		if expected != actual {
			return fmt.Errorf("generated master changed auxiliary table %q", object.Name)
		}
	}
	for _, suffix := range []string{"-wal", "-journal", "-shm"} {
		if _, err := os.Lstat(candidatePath + suffix); err == nil {
			return fmt.Errorf("generated master has an external SQLite journal")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func masterInsertSQL(table string, columns []MasterDbColumn) (string, error) {
	quoted, err := quoteMasterIdentifier(table)
	if err != nil {
		return "", err
	}
	names := make([]string, len(columns))
	parameters := make([]string, len(columns))
	for i, column := range columns {
		names[i], err = quoteMasterIdentifier(column.Name)
		if err != nil {
			return "", err
		}
		parameters[i] = "?"
	}
	return "INSERT INTO " + quoted + " (" + strings.Join(names, ",") + ") VALUES (" + strings.Join(parameters, ",") + ")", nil
}

func masterRowValues(row map[string]any, columns []MasterDbColumn) ([]any, error) {
	if len(row) != len(columns) {
		return nil, fmt.Errorf("master row must have exactly %d indexed and data columns", len(columns))
	}
	values := make([]any, len(columns))
	for i, column := range columns {
		value, found := row[column.Name]
		if !found {
			return nil, fmt.Errorf("master row is missing column %q", column.Name)
		}
		converted, err := NormalizeMasterValue(column, value)
		if err != nil {
			return nil, err
		}
		values[i] = converted
	}
	return values, nil
}

// Sum individual SHA-256 row hashes modulo 2^256 so verification is independent
// of SQLite's row iteration order and uses constant memory. Count also detects
// missing/extra rows. SQL constraints independently enforce key uniqueness.
type masterRowDigest struct {
	Sum   [32]byte
	Count int64
}

func (digest *masterRowDigest) add(values []any) error {
	hasher := sha256.New()
	for _, value := range values {
		if err := hashMasterValue(hasher, value); err != nil {
			return err
		}
	}
	bytes := hasher.Sum(nil)
	carry := uint16(0)
	for i := len(bytes) - 1; i >= 0; i-- {
		carry += uint16(digest.Sum[i]) + uint16(bytes[i])
		digest.Sum[i] = byte(carry)
		carry >>= 8
	}
	digest.Count++
	return nil
}

func hashMasterValue(hasher hash.Hash, value any) error {
	var prefix [9]byte
	var payload []byte
	switch value := value.(type) {
	case nil:
		prefix[0] = 0
	case int64:
		prefix[0] = 1
		payload = strconv.AppendInt(nil, value, 10)
	case float64:
		prefix[0] = 2
		payload = make([]byte, 8)
		binary.BigEndian.PutUint64(payload, math.Float64bits(value))
	case string:
		prefix[0] = 3
		payload = []byte(value)
	case []byte:
		prefix[0] = 4
		payload = value
	default:
		return fmt.Errorf("unsupported SQLite master value type %T", value)
	}
	binary.BigEndian.PutUint64(prefix[1:], uint64(len(payload)))
	hasher.Write(prefix[:])
	hasher.Write(payload)
	return nil
}
