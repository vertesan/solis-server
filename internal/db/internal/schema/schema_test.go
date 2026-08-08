package schema

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/vertesan/solis-server/internal/db/model/user/uname"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestIndexSpecsAreUniqueAndWellFormed verifies index names are unambiguous and
// every registered user collection has an index for its ownership filter.
func TestIndexSpecsAreUniqueAndWellFormed(t *testing.T) {
	specs := indexSpecs()
	if len(specs) != 81 {
		t.Fatalf("unexpected index count: got %d, want 81", len(specs))
	}
	seen := make(map[string]struct{}, len(specs))
	partials := make(map[string]any)
	queryIndexes := 0
	indexedDeletionKeys := make(map[string]map[string]bool)
	for _, spec := range specs {
		if len(spec.keys) == 0 {
			t.Fatalf("index %q has no keys", spec.name)
		}
		identity := fmt.Sprintf("%s.%s.%v", spec.database, spec.collection, spec.keys)
		if _, ok := seen[identity]; ok {
			t.Fatalf("duplicate index specification %s", identity)
		}
		seen[identity] = struct{}{}
		if spec.collection == uname.UserAuth && spec.partial != nil {
			partials[spec.name] = spec.partial
		}
		if !spec.unique {
			queryIndexes++
			if spec.name != "ix_SUID" {
				t.Fatalf("unexpected non-unique index %q", spec.name)
			}
		}
		if spec.database == "user" {
			if indexedDeletionKeys[spec.collection] == nil {
				indexedDeletionKeys[spec.collection] = make(map[string]bool)
			}
			indexedDeletionKeys[spec.collection][spec.keys[0].Key] = spec.unique
		}
	}
	if queryIndexes != 21 {
		t.Fatalf("non-unique query index count: got %d, want 21", queryIndexes)
	}
	for _, collection := range uname.Collections {
		if _, ok := indexedDeletionKeys[collection.Name][collection.SUIDField]; !ok {
			t.Fatalf("collection %q has no index beginning with deletion key %q", collection.Name, collection.SUIDField)
		}
	}
	for _, collection := range []string{uname.UserInvite, uname.UserActivityFanEventProgress, uname.UserStaffTrainingReset} {
		if !indexedDeletionKeys[collection]["SUID"] {
			t.Fatalf("FindOne collection %q must have a unique SUID index", collection)
		}
	}
	if len(partials) != 2 {
		t.Fatalf("userAuth partial index count: got %d, want 2", len(partials))
	}
	if got := partials["uq_refreshTokens"]; !reflect.DeepEqual(got, bson.M{"refreshTokens": bson.M{"$type": "string"}}) {
		t.Fatalf("refreshTokens partial filter = %#v, want string tokens only", got)
	}
	if got := partials["uq_oneTimeToken"]; !reflect.DeepEqual(got, bson.M{"oneTimeToken": bson.M{"$gt": ""}}) {
		t.Fatalf("oneTimeToken partial filter = %#v, want non-empty tokens only", got)
	}
}
