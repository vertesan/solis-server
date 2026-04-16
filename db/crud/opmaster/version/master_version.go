package version

import (
	"context"
	"errors"
	"fmt"
	"vertesan/solis-server/db/def/master/mname"
	"vertesan/solis-server/db/def/master/mstruct"
	"vertesan/solis-server/db/internal/depot"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// get master._version
func GetMasterVersion(ctx context.Context) (string, error) {
	var ver mstruct.Version
	err := depot.Master.Collection(mname.Version).FindOne(ctx, bson.M{}).Decode(&ver)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get master._version.version: %w", err)
	}
	return ver.Version, nil
}

// upsert master._version
func UpsertMasterVersion(ctx context.Context, version string) error {
	_, err := depot.Master.Collection(mname.Version).UpdateOne(
		ctx,
		bson.M{},
		bson.M{"$set": &mstruct.Version{Version: version}},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert master._version")
	}
	return nil
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

func CreateIndexToMasterDb(ctx context.Context, dbName string, indexes bson.D) error {
	if _, err := depot.Master.Collection(dbName).Indexes().CreateOne(
		ctx, mongo.IndexModel{Keys: indexes},
	); err != nil {
		return fmt.Errorf("failed to create index for master db %q: %w", dbName, err)
	}
	return nil
}
