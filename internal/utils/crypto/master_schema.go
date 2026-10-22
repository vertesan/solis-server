package crypto

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type MasterDbColumn struct {
	Name         string
	Type         string
	NotNull      bool
	DefaultValue *string
	PrimaryKey   int
	Hidden       int
}

type MasterDbSchemaObject struct {
	Type  string
	Name  string
	Table string
	SQL   string
}

// MasterDbSchema is trusted metadata read from the active encrypted pack; it
// must never be accepted from an editable YAML file.
type MasterDbSchema struct {
	Table    string
	Columns  []MasterDbColumn
	Objects  []MasterDbSchemaObject
	Settings map[string]string
	RowCount int64
}

// InspectMasterDb reads the application schema, including compound primary
// keys, from an existing encrypted pack without modifying it.
func InspectMasterDb(ctx context.Context, filePath, key, table string) (*MasterDbSchema, error) {
	db, err := openMasterDb(filePath, key, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return inspectMasterDb(ctx, db, table)
}

func inspectMasterDb(ctx context.Context, db *sql.DB, table string) (*MasterDbSchema, error) {
	quoted, err := quoteMasterIdentifier(table)
	if err != nil {
		return nil, err
	}
	result := &MasterDbSchema{Table: table, Settings: make(map[string]string)}
	rows, err := db.QueryContext(ctx, "SELECT type,name,tbl_name,coalesce(sql,'') FROM sqlite_master ORDER BY type,name")
	if err != nil {
		return nil, fmt.Errorf("read master schema: %w", err)
	}
	found := false
	for rows.Next() {
		var object MasterDbSchemaObject
		if err := rows.Scan(&object.Type, &object.Name, &object.Table, &object.SQL); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan master schema: %w", err)
		}
		if object.Type == "table" && object.Name == table {
			found = true
		}
		if strings.Contains(strings.ToUpper(object.SQL), "CREATE VIRTUAL TABLE") {
			rows.Close()
			return nil, fmt.Errorf("virtual master tables are unsupported: %q", object.Name)
		}
		if object.Type == "table" && strings.HasPrefix(object.Name, "sqlite_") && object.Name != "sqlite_sequence" {
			rows.Close()
			return nil, fmt.Errorf("unsupported internal SQLite table %q", object.Name)
		}
		result.Objects = append(result.Objects, object)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate master schema: %w", err)
	}
	rows.Close()
	if !found {
		return nil, fmt.Errorf("master table %q does not exist", table)
	}
	result.Columns, err = masterColumns(ctx, db, quoted)
	if err != nil {
		return nil, err
	}
	dataFound := false
	for _, column := range result.Columns {
		if column.Hidden != 0 {
			return nil, fmt.Errorf("generated or hidden master column %q is unsupported", column.Name)
		}
		dataFound = dataFound || column.Name == "data"
	}
	if !dataFound {
		return nil, fmt.Errorf("master table %q has no data column", table)
	}
	for _, name := range []string{
		"cipher_page_size", "cipher_use_hmac", "cipher_hmac_algorithm", "cipher_kdf_algorithm", "kdf_iter", "cipher_plaintext_header_size",
		"encoding", "user_version", "application_id", "auto_vacuum",
	} {
		var value string
		if err := db.QueryRowContext(ctx, "PRAGMA "+name).Scan(&value); err != nil {
			return nil, fmt.Errorf("read SQLite setting %s: %w", name, err)
		}
		result.Settings[name] = value
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+quoted).Scan(&result.RowCount); err != nil {
		return nil, fmt.Errorf("count master rows: %w", err)
	}
	return result, nil
}

func masterColumns(ctx context.Context, db *sql.DB, quoted string) ([]MasterDbColumn, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_xinfo("+quoted+")")
	if err != nil {
		return nil, fmt.Errorf("read master columns: %w", err)
	}
	defer rows.Close()
	var columns []MasterDbColumn
	for rows.Next() {
		var column MasterDbColumn
		var cid, notNull int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &column.Name, &column.Type, &notNull, &defaultValue, &column.PrimaryKey, &column.Hidden); err != nil {
			return nil, fmt.Errorf("scan master column: %w", err)
		}
		column.NotNull = notNull != 0
		if defaultValue.Valid {
			column.DefaultValue = &defaultValue.String
		}
		columns = append(columns, column)
	}
	return columns, rows.Err()
}
