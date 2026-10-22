package opmaster

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// IterateMasterPayloads keeps master exports bounded to one protobuf record at
// a time. Callers validate collection against the active manifest first.
func IterateMasterPayloads(ctx context.Context, collection string, consume func([]byte) error) error {
	cursor, err := depot.Master.Collection(collection).Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "data", Value: 1}}).SetBatchSize(128))
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		if cursor.Current.Lookup("data").Type != bson.TypeBinary {
			return fmt.Errorf("%s contains a missing or nonbinary data payload", collection)
		}
		var row struct {
			Data []byte `bson:"data"`
		}
		if err := cursor.Decode(&row); err != nil {
			return err
		}
		if err := consume(row.Data); err != nil {
			return fmt.Errorf("%s record: %w", collection, err)
		}
	}
	return cursor.Err()
}
