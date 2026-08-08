package version

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/internal/migration"
	"github.com/vertesan/solis-server/internal/db/model/master/mdoc"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type masterVersionState struct {
	Version        string `bson:"version"`
	ImportRevision int32  `bson:"importRevision"`
}

// get master._version
func GetMasterVersion(ctx context.Context) (string, error) {
	version, _, err := GetMasterVersionStatus(ctx)
	return version, err
}

// GetMasterVersionStatus returns the imported release version and whether it
// was written using the current master-import contract.
func GetMasterVersionStatus(ctx context.Context) (string, bool, error) {
	var state masterVersionState
	err := depot.Master.Collection(mname.Version).FindOne(ctx, bson.M{"_id": mdoc.VersionDocumentID}).Decode(&state)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to get master._version: %w", err)
	}
	return state.Version, state.ImportRevision == migration.CurrentMasterImportRevision, nil
}

// upsert master._version
func UpsertMasterVersion(ctx context.Context, version string) error {
	_, err := depot.Master.Collection(mname.Version).UpdateOne(
		ctx,
		bson.M{"_id": mdoc.VersionDocumentID},
		bson.M{"$set": &masterVersionState{
			Version:        version,
			ImportRevision: migration.CurrentMasterImportRevision,
		}},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert master._version: %w", err)
	}
	return nil
}

// ListMasterDbNames returns every collection currently present in the master
// database, including collections removed from the incoming release.
func ListMasterDbNames(ctx context.Context) ([]string, error) {
	names, err := depot.Master.ListCollectionNames(ctx, bson.M{"type": "collection"})
	if err != nil {
		return nil, fmt.Errorf("failed to list master database collections: %w", err)
	}
	return names, nil
}

func DeleteMasterDb(ctx context.Context, tagType string) error {
	if _, err := depot.Master.Collection(tagType).DeleteMany(ctx, bson.M{}); err != nil {
		return fmt.Errorf("failed to delete master db %q: %w", tagType, err)
	}
	return nil
}

func InsertMasterDb(ctx context.Context, tagType string, data []map[string]interface{}) error {
	if len(data) == 0 {
		return nil
	}
	if _, err := depot.Master.Collection(tagType).InsertMany(ctx, data); err != nil {
		return fmt.Errorf("failed to insert master db %q: %w", tagType, err)
	}
	return nil
}

// CreateIndexesToMasterDb ensures a full query index and a managed partial
// unique index exist for a master collection's natural key.
func CreateIndexesToMasterDb(ctx context.Context, dbName string, indexes bson.D) error {
	models := masterNaturalKeyIndexModels(indexes)
	hasQueryIndex, err := hasFullMasterNaturalKeyIndex(ctx, dbName, indexes)
	if err != nil {
		return err
	}
	if !hasQueryIndex {
		if _, err := depot.Master.Collection(dbName).Indexes().CreateOne(ctx, models[0]); err != nil {
			// A concurrent or legacy creator may have installed an equivalent
			// full index under a different name between the check and create.
			hasQueryIndex, checkErr := hasFullMasterNaturalKeyIndex(ctx, dbName, indexes)
			if checkErr != nil {
				return checkErr
			}
			if !hasQueryIndex {
				return fmt.Errorf("failed to create query index for master db %q: %w", dbName, err)
			}
		}
	}
	if _, err := depot.Master.Collection(dbName).Indexes().CreateOne(ctx, models[1]); err != nil {
		return fmt.Errorf("failed to create unique index for master db %q: %w", dbName, err)
	}
	return nil
}

// hasFullMasterNaturalKeyIndex reports whether an equivalent ordinary query
// index already exists, regardless of its name.
func hasFullMasterNaturalKeyIndex(ctx context.Context, dbName string, indexes bson.D) (bool, error) {
	desiredKeys, err := bson.Marshal(indexes)
	if err != nil {
		return false, fmt.Errorf("failed to encode master index keys: %w", err)
	}
	var specifications []struct {
		Keys                    bson.Raw `bson:"key"`
		Unique                  bool     `bson:"unique"`
		Sparse                  bool     `bson:"sparse"`
		Hidden                  bool     `bson:"hidden"`
		PartialFilterExpression bson.Raw `bson:"partialFilterExpression"`
		Collation               bson.Raw `bson:"collation"`
	}
	cursor, err := depot.Master.Collection(dbName).Indexes().List(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to list indexes for master db %q: %w", dbName, err)
	}
	if err := cursor.All(ctx, &specifications); err != nil {
		return false, fmt.Errorf("failed to decode indexes for master db %q: %w", dbName, err)
	}
	for _, specification := range specifications {
		if !specification.Unique &&
			!specification.Sparse &&
			!specification.Hidden &&
			len(specification.PartialFilterExpression) == 0 &&
			len(specification.Collation) == 0 &&
			bytes.Equal(specification.Keys, desiredKeys) {
			return true, nil
		}
	}
	return false, nil
}

// MasterNaturalKeyMarker returns the deterministic marker stored on documents
// covered by the managed unique index for indexes.
func MasterNaturalKeyMarker(indexes bson.D) string {
	return "uq_master_natural_key_" + masterNaturalKeySpecID(indexes)
}

// masterNaturalKeySpecID hashes an ordered natural-key specification into a
// stable identifier used by managed index names.
func masterNaturalKeySpecID(indexes bson.D) string {
	hash := sha256.New()
	for _, index := range indexes {
		_, _ = hash.Write([]byte(index.Key))
		_, _ = hash.Write([]byte{0})
		_, _ = fmt.Fprint(hash, index.Value)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// masterNaturalKeyIndexModels builds the ordinary query index and the partial
// unique index used for newly imported master documents.
func masterNaturalKeyIndexModels(indexes bson.D) []mongo.IndexModel {
	marker := MasterNaturalKeyMarker(indexes)
	return []mongo.IndexModel{
		{
			Keys:    indexes,
			Options: options.Index().SetName("ix_master_natural_key_" + masterNaturalKeySpecID(indexes)),
		},
		{
			Keys: indexes,
			Options: options.Index().
				SetName(marker).
				SetUnique(true).
				SetPartialFilterExpression(bson.M{mdoc.MasterNaturalKeyField: marker}),
		},
	}
}
