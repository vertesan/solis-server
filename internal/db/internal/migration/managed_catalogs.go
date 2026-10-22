package migration

import (
	"context"
	"errors"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ensureManagedCatalogs initializes newly externalized catalogs without replaying
// level 1 or replacing administrator changes in existing installations. Seed
// files are read only when the corresponding document/legacy field is missing.
func ensureManagedCatalogs(ctx context.Context) error {
	if err := ensureCatalogSeed(ctx, sname.GachaRules, "gacha-rules/gacha_rules.yaml", new(smodel.GachaRules)); err != nil {
		return err
	}
	if err := ensureCatalogSeed(ctx, sname.GachaCatalog, "gacha/gacha_catalog.yaml", new(smodel.GachaCatalog)); err != nil {
		return err
	}

	// Explicit null represents a cleared notice and must not be seeded again.
	// A completely absent catalog is also left alone instead of restoring a
	// catalog that an administrator may deliberately have removed.
	collection := depot.Solis.Collection(sname.NoticeCatalog)
	filter := bson.M{"showcaseShopNotice": bson.M{"$exists": false}}
	var legacy bson.M
	err := collection.FindOne(ctx, filter, options.FindOne().SetProjection(bson.M{"_id": 1})).Decode(&legacy)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed checking showcase notice initialization: %w", err)
	}
	notice := new(papi.NoticeInfo)
	if err := unmarshalCatalogProtoYAML("notice/showcase_shop_notice.yaml", notice); err != nil {
		return err
	}
	filter["_id"] = legacy["_id"]
	_, err = collection.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"showcaseShopNotice": notice}})
	return err
}

func ensureCatalogSeed(ctx context.Context, collectionName, filename string, seed interface{ Validate() error }) error {
	collection := depot.Solis.Collection(collectionName)
	err := collection.FindOne(ctx, bson.M{}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	if err == nil {
		return nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("failed checking %s initialization: %w", collectionName, err)
	}
	if err := unmarshalCatalogYAML(filename, seed); err != nil {
		return err
	}
	if err := seed.Validate(); err != nil {
		return fmt.Errorf("invalid initial catalog %q: %w", filename, err)
	}
	data, err := bson.Marshal(seed)
	if err != nil {
		return err
	}
	var document bson.M
	if err := bson.Unmarshal(data, &document); err != nil {
		return err
	}
	// A stable ID prevents concurrent first starts from inserting two singleton
	// seeds, while $setOnInsert preserves a catalog created during the read.
	document["_id"] = collectionName
	_, err = collection.UpdateOne(ctx, bson.M{}, bson.M{"$setOnInsert": document}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("failed initializing %s: %w", collectionName, err)
	}
	return nil
}
