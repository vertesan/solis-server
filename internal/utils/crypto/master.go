package crypto

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

// DecryptMasterDbFile retains the legacy batch API. New callers should use
// IterateMasterDb so large master packs need not be retained in memory.
func DecryptMasterDbFile(dbName, filePath, key string) ([]map[string]interface{}, error) {
	var results []map[string]any
	err := IterateMasterDb(context.Background(), filePath, key, dbName, func(row map[string]any) error {
		results = append(results, row)
		return nil
	})
	return results, err
}

// IterateMasterDb opens an existing encrypted pack read-only. Each row and its
// byte slices belong to the callback and remain valid after it returns.
func IterateMasterDb(ctx context.Context, filePath, key, table string, visit func(map[string]any) error) error {
	if visit == nil {
		return fmt.Errorf("master row callback is required")
	}
	db, err := openMasterDb(filePath, key, true)
	if err != nil {
		return err
	}
	defer db.Close()
	return iterateMasterRows(ctx, db, table, visit)
}

func openMasterDb(filePath, key string, readOnly bool) (*sql.DB, error) {
	decodedKey, err := hex.DecodeString(key)
	if err != nil || len(decodedKey) != 32 {
		return nil, fmt.Errorf("master encryption key must contain 32 hexadecimal bytes")
	}
	filePath, err = filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("resolve master database path: %w", err)
	}
	if readOnly {
		info, err := os.Lstat(filePath)
		if err != nil {
			return nil, fmt.Errorf("stat master database: %w", err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("master database must be a regular file")
		}
	}
	uri := url.URL{Scheme: "file", Path: filePath}
	query := url.Values{"_pragma_key": {"x'" + key + "'"}, "_busy_timeout": {"5000"}}
	if readOnly {
		query.Set("mode", "ro")
	} else {
		query.Set("mode", "rw")
	}
	uri.RawQuery = query.Encode()
	db, err := sql.Open("sqlite3", uri.String())
	if err != nil {
		// Do not include the URI: it contains the encryption key.
		return nil, fmt.Errorf("open master database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}

func quoteMasterIdentifier(identifier string) (string, error) {
	if identifier == "" || strings.ContainsRune(identifier, 0) {
		return "", fmt.Errorf("invalid empty or NUL-containing SQLite identifier")
	}
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`, nil
}

func iterateMasterRows(ctx context.Context, db *sql.DB, table string, visit func(map[string]any) error) error {
	quoted, err := quoteMasterIdentifier(table)
	if err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+quoted)
	if err != nil {
		return fmt.Errorf("query master table %q: %w", table, err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("read master column names: %w", err)
	}
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return fmt.Errorf("scan master row: %w", err)
		}
		row := make(map[string]any, len(columns))
		for i, name := range columns {
			if value, ok := values[i].([]byte); ok {
				copied := make([]byte, len(value))
				copy(copied, value)
				row[name] = copied
			} else {
				row[name] = values[i]
			}
		}
		if err := visit(row); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate master table %q: %w", table, err)
	}
	return nil
}
