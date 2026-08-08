package umodel

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestUserAuthUsesCanonicalOneTimeTokenField guards the BSON field spelling
// used by token issuance, lookup, consumption, and indexing.
func TestUserAuthUsesCanonicalOneTimeTokenField(t *testing.T) {
	data, err := bson.Marshal(&UserAuth{OnetimeToken: "token"})
	if err != nil {
		t.Fatalf("marshal UserAuth: %v", err)
	}
	raw := bson.Raw(data)
	if got := raw.Lookup("oneTimeToken").StringValue(); got != "token" {
		t.Fatalf("oneTimeToken = %q, want token", got)
	}
	if value := raw.Lookup("onetimeToken"); value.Type != 0 {
		t.Fatalf("legacy onetimeToken field is still encoded: %v", value)
	}
}
