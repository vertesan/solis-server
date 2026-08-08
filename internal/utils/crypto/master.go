package crypto

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/vertesan/solis-server/internal/db/model/master/mdoc"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	mversion "github.com/vertesan/solis-server/internal/db/store/opmaster/version"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"

	_ "github.com/mutecomm/go-sqlcipher/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

type preparedMaster struct {
	name      string
	documents []map[string]any
	indexKeys bson.D
}

type MasterFileError struct {
	FilePath string
	Err      error
}

// Error describes which cached master file could not be prepared.
func (e *MasterFileError) Error() string {
	return fmt.Sprintf("failed to prepare master database file %q: %v", e.FilePath, e.Err)
}

// Unwrap exposes the underlying decrypt, validation, or read error.
func (e *MasterFileError) Unwrap() error {
	return e.Err
}

// UpdateAllMaster prepares and validates an entire release before atomically
// replacing its packs, collections, profile version, and import marker.
func UpdateAllMaster(ctx context.Context, expectedProfileVersion string, masterTag *ptransaction.MasterTag) error {
	prepared, err := prepareAllMaster(ctx, masterTag)
	if err != nil {
		return err
	}
	existingCollections, err := mversion.ListMasterDbNames(ctx)
	if err != nil {
		return err
	}
	collections := masterCollections(existingCollections, prepared)

	// Index creation is not allowed in a transaction. Create every required
	// additive index before any release data can be committed.
	zap.L().Info("creating indexes for master database, this may take a few minutes for the first startup...")
	for _, item := range prepared {
		if err := mversion.CreateIndexesToMasterDb(ctx, item.name, item.indexKeys); err != nil {
			return err
		}
	}

	zap.L().Info("updating master database from storage")
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		profile, err := opsolis.FindServerProfile(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to revalidate server profile: %w", err)
		}
		if profile.MasterVersion != expectedProfileVersion {
			return nil, fmt.Errorf(
				"server profile master version changed while preparing update: expected %q, got %q",
				expectedProfileVersion,
				profile.MasterVersion,
			)
		}

		if err := opsolis.ReplaceAllMasterTagPacks(ctx, masterTag.MasterTagPacks); err != nil {
			return nil, err
		}
		for _, collection := range collections {
			if err := mversion.DeleteMasterDb(ctx, collection); err != nil {
				return nil, err
			}
		}
		for _, item := range prepared {
			if err := mversion.InsertMasterDb(ctx, item.name, item.documents); err != nil {
				return nil, err
			}
		}
		if err := opsolis.UpdateServerProfileMasterVersion(ctx, masterTag.Version); err != nil {
			return nil, err
		}
		if err := mversion.UpsertMasterVersion(ctx, masterTag.Version); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("failed to update master db: %w", err)
	}

	zap.L().Info("master database update completed")
	return nil
}

// prepareAllMaster validates pack metadata and fully decrypts every SQLite pack
// into MongoDB documents before the release transaction starts.
func prepareAllMaster(ctx context.Context, masterTag *ptransaction.MasterTag) ([]preparedMaster, error) {
	if masterTag == nil {
		return nil, fmt.Errorf("master tag is nil")
	}
	if masterTag.Version == "" {
		return nil, fmt.Errorf("master tag version is empty")
	}
	if len(masterTag.MasterTagPacks) == 0 {
		return nil, fmt.Errorf("master tag contains no packs")
	}

	tags := append([]*ptransaction.MasterTagPack(nil), masterTag.MasterTagPacks...)
	sort.Slice(tags, func(i, j int) bool {
		if tags[i] == nil {
			return false
		}
		if tags[j] == nil {
			return true
		}
		return tags[i].Type < tags[j].Type
	})

	prepared := make([]preparedMaster, 0, len(tags))
	for i, tag := range tags {
		if tag == nil {
			return nil, fmt.Errorf("master tag pack at index %d is nil", i)
		}
		if tag.Type == "" {
			return nil, fmt.Errorf("master tag pack at index %d has an empty type", i)
		}
		if tag.Type == mname.Version {
			return nil, fmt.Errorf("master tag pack type %q is reserved", tag.Type)
		}
		if i > 0 && tags[i-1] != nil && tags[i-1].Type == tag.Type {
			return nil, fmt.Errorf("duplicate master tag pack type %q", tag.Type)
		}
		if tag.FileName == "" || path.Base(tag.FileName) != tag.FileName {
			return nil, fmt.Errorf("master tag pack %q has an invalid file name %q", tag.Type, tag.FileName)
		}
		if tag.CryptoKey == "" {
			return nil, fmt.Errorf("master tag pack %q has an empty crypto key", tag.Type)
		}
	}

	for _, tag := range tags {
		filePath := path.Join(config.GetDBDir(), tag.FileName)
		documents, indexKeys, err := decryptToSlice(ctx, tag.Type, filePath, tag.CryptoKey)
		if err != nil {
			return nil, &MasterFileError{
				FilePath: filePath,
				Err:      fmt.Errorf("failed to decrypt master db %q: %w", tag.Type, err),
			}
		}
		prepared = append(prepared, preparedMaster{
			name:      tag.Type,
			documents: documents,
			indexKeys: indexKeys,
		})
	}
	return prepared, nil
}

// masterCollections returns a sorted union of existing and incoming master
// collections so stale collections are cleared during replacement.
func masterCollections(existingCollections []string, prepared []preparedMaster) []string {
	seen := make(map[string]struct{}, len(existingCollections)+len(prepared))
	for _, collection := range existingCollections {
		if collection != "" && collection != mname.Version && !strings.HasPrefix(collection, "system.") {
			seen[collection] = struct{}{}
		}
	}
	for _, item := range prepared {
		seen[item.name] = struct{}{}
	}
	collections := make([]string, 0, len(seen))
	for collection := range seen {
		collections = append(collections, collection)
	}
	sort.Strings(collections)
	return collections
}

// decryptToSlice reads every row from an encrypted SQLite master table,
// validates its protobuf payload and natural keys, and returns import documents
// plus the deterministic MongoDB natural-key index specification.
func decryptToSlice(ctx context.Context, dbName string, filePath string, key string) ([]map[string]any, bson.D, error) {
	uri := fmt.Sprintf("%s?_pragma_key=x'%s'", filePath, key)
	db, err := sql.Open("sqlite3", uri)
	if err != nil {
		return nil, nil, fmt.Errorf("failed opening db: %w", err)
	}
	defer db.Close()
	quotedTable := `"` + strings.ReplaceAll(dbName, `"`, `""`) + `"`
	rows, err := db.QueryContext(ctx, "select * from "+quotedTable)
	if err != nil {
		return nil, nil, fmt.Errorf("failed querying db: %w", err)
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get column names: %w", err)
	}
	hasData := false
	naturalKeys := make([]string, 0, len(colNames))
	for _, name := range colNames {
		if name == mdoc.MasterNaturalKeyField {
			return nil, nil, fmt.Errorf("master table %q uses reserved column %q", dbName, name)
		}
		if name == "data" {
			hasData = true
			continue
		}
		naturalKeys = append(naturalKeys, name)
	}
	if !hasData {
		return nil, nil, fmt.Errorf("master table %q has no data column", dbName)
	}
	if len(naturalKeys) == 0 {
		return nil, nil, fmt.Errorf("master table %q has no natural-key columns", dbName)
	}
	sort.Strings(naturalKeys)
	indexKeys := make(bson.D, 0, len(naturalKeys))
	for _, name := range naturalKeys {
		indexKeys = append(indexKeys, bson.E{Key: name, Value: 1})
	}
	masterNaturalKeyMarker := mversion.MasterNaturalKeyMarker(indexKeys)

	results := make([]map[string]any, 0)
	seenNaturalKeys := make(map[string]int)

	for rows.Next() {
		values := make([]any, len(colNames))
		valuePtrs := make([]any, len(colNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err = rows.Scan(valuePtrs...); err != nil {
			return nil, nil, fmt.Errorf("failed to scan row: %w", err)
		}
		rowMap := make(map[string]any, len(colNames))
		for i, name := range colNames {
			val := values[i]
			if b, ok := val.([]byte); ok && name != "data" {
				rowMap[name] = string(b)
			} else {
				rowMap[name] = val
			}
		}
		if _, ok := rowMap["data"].([]byte); !ok {
			return nil, nil, fmt.Errorf("row %d has a non-binary data column", len(results)+1)
		}
		naturalKey := make(bson.D, 0, len(naturalKeys))
		for _, name := range naturalKeys {
			naturalKey = append(naturalKey, bson.E{Key: name, Value: rowMap[name]})
		}
		encodedKey, err := bson.Marshal(naturalKey)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to encode natural key: %w", err)
		}
		if previousRow, exists := seenNaturalKeys[string(encodedKey)]; exists {
			return nil, nil, fmt.Errorf("duplicate natural key in rows %d and %d", previousRow, len(results)+1)
		}
		seenNaturalKeys[string(encodedKey)] = len(results) + 1
		rowMap[mdoc.MasterNaturalKeyField] = masterNaturalKeyMarker
		results = append(results, rowMap)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed while reading rows: %w", err)
	}
	return results, indexKeys, nil
}
