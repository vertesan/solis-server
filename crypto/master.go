package crypto

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"path"
	mversion "vertesan/solis-server/db/store/opmaster/version"
	"vertesan/solis-server/db/store/opsolis"
	"vertesan/solis-server/db/transaction"
	"vertesan/solis-server/utils/config"
	"vertesan/solis-server/utils/rich"

	_ "github.com/mutecomm/go-sqlcipher/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

func CheckAndUpdateAllMaster(profileVersion string) error {
	ver, err := mversion.GetMasterVersion(context.Background())
	if err != nil {
		return err
	}
	rich.Info("master database version: %s", ver)
	// check current version vs version string in serverProfile
	if ver == profileVersion {
		return nil
	}
	rich.Info("updating master database from storage")
	// get all masterTagPacks
	tags, err := opsolis.GetAllMasterTagPack(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get master tag pack: %w", err)
	}
	// prepare an indexMap for creating indexes after transaction
	indexMap := make(map[string]bson.D)

	// start a transaction to migrate master databases from SQLite to MongoDB
	err = transaction.WithTransaction(func(ctx context.Context) (interface{}, error) {
		for _, tag := range tags {
			filePath := path.Join(config.GetDBDir(), tag.FileName)
			data, err := decryptToSlice(tag.Type, filePath, tag.CryptoKey)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt db: %w", err)
			}
			// create indexMap
			if len(data) > 0 {
				var indexes bson.D
				for key := range maps.Keys(data[0]) {
					if key != "data" {
						indexes = append(indexes, bson.E{Key: key, Value: 1})
					}
				}
				indexMap[tag.Type] = indexes
			}
			// update db
			if err := mversion.DeleteMasterDb(ctx, tag.Type); err != nil {
				return nil, err
			}
			if err := mversion.InsertMasterDb(ctx, tag.Type, data); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}, readconcern.Local(), writeconcern.W1())
	if err != nil {
		return fmt.Errorf("failed to update master db: %w", err)
	}

	// creating indexes is not allowed in a transaction, we have to do this separately
	rich.Info("creating indexes for master database, this may take a few minutes for the first startup...")
	for k, v := range indexMap {
		if err := mversion.CreateIndexToMasterDb(context.Background(), k, v); err != nil {
			return err
		}
	}

	// finally, upsert master version in master._version
	if err := mversion.UpsertMasterVersion(context.Background(), profileVersion); err != nil {
		return err
	}

	rich.Info("master database update completed")
	return nil
}

func decryptToSlice(dbName string, filePath string, key string) ([]map[string]interface{}, error) {
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
