package opsolis

import (
	"context"
	"errors"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"
	"github.com/vertesan/solis-server/internal/proto/octo"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func FindManifestMetaInfo(ctx context.Context) (*smodel.OctoManifest, error) {
	return scommon.FindOne[smodel.OctoManifest](
		ctx,
		sname.OctoManifest,
		bson.M{},
		options.FindOne().SetProjection(bson.M{"revision": 1, "urlFormat": 1}),
	)
}

func FindFullManifest(ctx context.Context) (*octo.Database, error) {
	return scommon.FindOne[octo.Database](
		ctx,
		sname.OctoManifest,
		bson.M{},
	)
}

func FindManifestObject(ctx context.Context, objectName string, objectType string) (*octo.Data, error) {
	var objectListName string
	switch objectType {
	case "assetbundle":
		objectListName = "$assetBundleList"
	case "resources":
		objectListName = "$resourceList"
	default:
		return nil, errors.New("unknown object type: object type must be \"assetbundle\" or \"resources\"")
	}

	var data octo.Data
	pipeline := []bson.M{
		{
			"$addFields": bson.M{
				"matchedObject": bson.M{
					"$first": bson.M{
						"$filter": bson.M{
							"input": objectListName,
							"cond": bson.M{
								"$eq": []interface{}{"$$this.objectName", objectName},
							},
						},
					},
				},
			},
		},
		{
			"$replaceRoot": bson.M{
				"newRoot": "$matchedObject",
			},
		},
	}
	cursor, err := depot.Solis.Collection(sname.OctoManifest).Aggregate(
		ctx,
		pipeline,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate octo data: %w", err)
	}
	defer cursor.Close(ctx)

	if cursor.Next(ctx) {
		if err := cursor.Decode(&data); err != nil {
			return nil, fmt.Errorf("failed to decode octo data: %w", err)
		}
		return &data, nil
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("failed while aggregating octo data: %w", err)
	}
	return nil, fmt.Errorf("manifest object %q (%s): %w", objectName, objectType, mongo.ErrNoDocuments)
}

func DeleteAllManifests(ctx context.Context) error {
	return scommon.Delete(
		ctx,
		sname.OctoManifest,
		bson.M{},
	)
}

func InsertManifest(ctx context.Context, octodb *octo.Database) error {
	return scommon.InsertOne(
		ctx,
		sname.OctoManifest,
		octodb,
	)
}
