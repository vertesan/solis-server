package crypto

import (
	"database/sql"
	"fmt"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

func DecryptMasterDbFile(dbName string, filePath string, key string) ([]map[string]interface{}, error) {
	uri := fmt.Sprintf("%s?_pragma_key=x'%s'", filePath, key)
	db, err := sql.Open("sqlite3", uri)
	if err != nil {
		return nil, fmt.Errorf("failed opening db: %w", err)
	}
	defer db.Close()
	rows, err := db.Query(fmt.Sprintf("select * from %s;", dbName))
	if err != nil {
		return nil, fmt.Errorf("failed querying db: %w", err)
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get column names: %w", err)
	}
	var results []map[string]any

	for rows.Next() {
		values := make([]interface{}, len(colNames))
		valuePtrs := make([]interface{}, len(colNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err = rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		rowMap := make(map[string]interface{})
		for i, name := range colNames {
			val := values[i]
			if b, ok := val.([]byte); ok && name != "data" {
				rowMap[name] = string(b)
			} else {
				rowMap[name] = val
			}
		}
		results = append(results, rowMap)
	}
	return results, nil
}
