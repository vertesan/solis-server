package crypto

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var sqliteDecimal = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

// NormalizeMasterValue applies the trusted SQLite column's storage affinity.
// In particular, the official schema uses STRING, which has NUMERIC affinity,
// not TEXT: the protobuf ID "-001" is stored as INTEGER -1 in GachaExchange.
// Keep the protobuf unchanged, but derive identical SQL and Mongo index values.
func NormalizeMasterValue(column MasterDbColumn, value any) (any, error) {
	value, err := driver.DefaultParameterConverter.ConvertValue(value)
	if err != nil {
		return nil, fmt.Errorf("invalid value for master column %q: %w", column.Name, err)
	}
	if boolean, ok := value.(bool); ok {
		value = int64(0)
		if boolean {
			value = int64(1)
		}
	}
	columnType := strings.ToUpper(column.Type)
	affinity := "NUMERIC"
	switch {
	case strings.Contains(columnType, "INT"):
		affinity = "INTEGER"
	case strings.Contains(columnType, "CHAR"), strings.Contains(columnType, "CLOB"), strings.Contains(columnType, "TEXT"):
		affinity = "TEXT"
	case columnType == "", strings.Contains(columnType, "BLOB"):
		affinity = "BLOB"
	case strings.Contains(columnType, "REAL"), strings.Contains(columnType, "FLOA"), strings.Contains(columnType, "DOUB"):
		affinity = "REAL"
	}
	if number, ok := value.(float64); ok && math.IsNaN(number) {
		return nil, fmt.Errorf("NaN indexed value is unsupported for master column %q", column.Name)
	}
	if affinity == "BLOB" {
		return value, nil
	}
	if affinity == "TEXT" {
		switch number := value.(type) {
		case int64:
			return strconv.FormatInt(number, 10), nil
		case float64:
			// SQLite's floating-point text formatting differs from Go's.
			return sqliteStoredValue(affinity, value)
		default:
			return value, nil
		}
	}
	switch number := value.(type) {
	case int64:
		if affinity == "REAL" {
			return float64(number), nil
		}
		return number, nil
	case float64:
		// Match SQLite's sqlite3VdbeIntegerAffinity: boundary values are
		// deliberately excluded, even where a float->int cast is possible.
		if affinity != "REAL" && number > float64(math.MinInt64) && number < float64(math.MaxInt64) && math.Trunc(number) == number {
			return int64(number), nil
		}
		return number, nil
	case string:
		trimmed := strings.Trim(number, " \t\n\v\f\r")
		if integer, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			if affinity == "REAL" {
				return float64(integer), nil
			}
			return integer, nil
		}
		if !sqliteDecimal.MatchString(trimmed) {
			return number, nil
		}
		// Rare real/exponent/overflow strings use SQLite itself, preserving its
		// exact parser and rounding behavior instead of approximating it in Go.
		return sqliteStoredValue(affinity, number)
	default:
		return value, nil // NULL and BLOB values are not affinity-converted.
	}
}

func sqliteStoredValue(affinity string, value any) (any, error) {
	// affinity is selected only from the fixed constants above, never SQL from
	// a YAML file. This one-row database lives entirely in memory.
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("CREATE TABLE scalar (value " + affinity + ")"); err != nil {
		return nil, err
	}
	if _, err := db.Exec("INSERT INTO scalar(value) VALUES (?)", value); err != nil {
		return nil, err
	}
	var result any
	if err := db.QueryRow("SELECT value FROM scalar").Scan(&result); err != nil {
		return nil, err
	}
	return result, nil
}
