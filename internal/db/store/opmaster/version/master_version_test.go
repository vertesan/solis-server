package version

import (
	"reflect"
	"testing"

	"github.com/vertesan/solis-server/internal/db/model/master/mdoc"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestMasterNaturalKeyIndexNameIsStablePerKeySpec verifies index markers are
// deterministic and change when the ordered natural-key definition changes.
func TestMasterNaturalKeyIndexNameIsStablePerKeySpec(t *testing.T) {
	keys := bson.D{{Key: "first", Value: 1}, {Key: "second", Value: 1}}
	got := MasterNaturalKeyMarker(keys)
	if again := MasterNaturalKeyMarker(keys); got != again {
		t.Fatalf("index name is not stable: first %q, second %q", got, again)
	}
	if changed := MasterNaturalKeyMarker(bson.D{{Key: "second", Value: 1}, {Key: "first", Value: 1}}); got == changed {
		t.Fatalf("different ordered key specs received the same index name %q", got)
	}

	models := masterNaturalKeyIndexModels(keys)
	if len(models) != 2 {
		t.Fatalf("unexpected index model count: got %d, want 2", len(models))
	}
	if !reflect.DeepEqual(models[0].Keys, keys) || !reflect.DeepEqual(models[1].Keys, keys) {
		t.Fatalf("index models do not preserve deterministic keys: %#v", models)
	}
	queryOptions := new(options.IndexOptions)
	for _, apply := range models[0].Options.List() {
		if err := apply(queryOptions); err != nil {
			t.Fatalf("failed to apply query index option: %v", err)
		}
	}
	wantQueryName := "ix_master_natural_key_" + masterNaturalKeySpecID(keys)
	if queryOptions.Name == nil || *queryOptions.Name != wantQueryName {
		t.Fatalf("unexpected query index name: %v", queryOptions.Name)
	}
	if queryOptions.Unique != nil || queryOptions.PartialFilterExpression != nil {
		t.Fatalf("query index is not full and non-unique: %#v", queryOptions)
	}
	model := models[1]
	indexOptions := new(options.IndexOptions)
	for _, apply := range model.Options.List() {
		if err := apply(indexOptions); err != nil {
			t.Fatalf("failed to apply index option: %v", err)
		}
	}
	if indexOptions.Name == nil || *indexOptions.Name != got {
		t.Fatalf("unexpected index name: %v", indexOptions.Name)
	}
	if indexOptions.Unique == nil || !*indexOptions.Unique {
		t.Fatal("master natural-key index is not unique")
	}
	wantFilter := bson.M{mdoc.MasterNaturalKeyField: got}
	if filter, ok := indexOptions.PartialFilterExpression.(bson.M); !ok || filter[mdoc.MasterNaturalKeyField] != wantFilter[mdoc.MasterNaturalKeyField] {
		t.Fatalf("unexpected partial filter: %#v", indexOptions.PartialFilterExpression)
	}
}
