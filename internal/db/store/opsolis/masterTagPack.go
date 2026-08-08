package opsolis

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis/internal/scommon"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func UpsertMasterTagPack(ctx context.Context, masterTagPack *ptransaction.MasterTagPack) error {
	return scommon.UpsertOne(
		ctx,
		sname.MasterTagPack,
		bson.M{"type": masterTagPack.Type},
		masterTagPack,
	)
}

// ReplaceAllMasterTagPacks atomically makes the stored pack set exactly match
// masterTagPacks, including removal of pack types absent from the new release.
func ReplaceAllMasterTagPacks(ctx context.Context, masterTagPacks []*ptransaction.MasterTagPack) error {
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		collection := depot.Solis.Collection(sname.MasterTagPack)
		if _, err := collection.DeleteMany(ctx, bson.M{}); err != nil {
			return nil, fmt.Errorf("failed to delete master tag packs: %w", err)
		}
		if len(masterTagPacks) == 0 {
			return nil, nil
		}
		if _, err := collection.InsertMany(ctx, masterTagPacks); err != nil {
			return nil, fmt.Errorf("failed to insert master tag packs: %w", err)
		}
		return nil, nil
	})
}

// GetAllMasterTagPackTypes reads only the stored type fields used to determine
// whether a complete master release repair is required.
func GetAllMasterTagPackTypes(ctx context.Context) ([]string, error) {
	cursor, err := depot.Solis.Collection(sname.MasterTagPack).Find(
		ctx,
		bson.M{},
		options.Find().SetProjection(bson.M{"_id": 0, "type": 1}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find master tag pack types: %w", err)
	}
	defer cursor.Close(ctx)
	var types []string
	for cursor.Next(ctx) {
		value := cursor.Current.Lookup("type")
		if value.Type == bson.TypeString {
			types = append(types, value.StringValue())
		} else {
			// Preserve the row count so malformed/missing types force an exact
			// replacement rather than blocking the legacy repair path.
			types = append(types, "")
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("failed while reading master tag pack types: %w", err)
	}
	return types, nil
}

func GetAllMasterTagPack(ctx context.Context) ([](*ptransaction.MasterTagPack), error) {
	return scommon.FindAll[ptransaction.MasterTagPack](
		ctx,
		sname.MasterTagPack,
		bson.M{},
	)
}
