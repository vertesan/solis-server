package transaction

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

// TestTransactionOptionsAreSnapshotAndDurable guards the centralized ACID
// options applied to every application transaction.
func TestTransactionOptionsAreSnapshotAndDurable(t *testing.T) {
	built := &options.TransactionOptions{}
	for _, apply := range transactionOptions().List() {
		if err := apply(built); err != nil {
			t.Fatalf("apply transaction option: %v", err)
		}
	}
	if built.ReadConcern == nil || built.ReadConcern.Level != readconcern.Snapshot().Level {
		t.Fatalf("read concern = %#v, want snapshot", built.ReadConcern)
	}
	if built.ReadPreference == nil || built.ReadPreference.Mode() != readpref.PrimaryMode {
		t.Fatalf("read preference = %#v, want primary", built.ReadPreference)
	}
	if built.WriteConcern == nil || built.WriteConcern.W != writeconcern.WCMajority {
		t.Fatalf("write concern = %#v, want majority", built.WriteConcern)
	}
	if built.WriteConcern.Journal == nil || !*built.WriteConcern.Journal {
		t.Fatalf("write concern = %#v, want journaling enabled", built.WriteConcern)
	}
}
