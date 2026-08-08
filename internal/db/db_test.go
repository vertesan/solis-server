package db

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

// TestClientOptionsUseDurableConcerns verifies URI settings cannot weaken the
// service's read preference, read concern, or write durability.
func TestClientOptionsUseDurableConcerns(t *testing.T) {
	opts := clientOptions("mongodb://localhost:27017/?readConcernLevel=local&w=1&journal=false&readPreference=secondary")
	if opts.ReadConcern == nil || opts.ReadConcern.Level != readconcern.Majority().Level {
		t.Fatalf("read concern = %#v, want majority", opts.ReadConcern)
	}
	if opts.ReadPreference == nil || opts.ReadPreference.Mode() != readpref.PrimaryMode {
		t.Fatalf("read preference = %#v, want primary", opts.ReadPreference)
	}
	if opts.WriteConcern == nil || opts.WriteConcern.W != writeconcern.WCMajority {
		t.Fatalf("write concern = %#v, want majority", opts.WriteConcern)
	}
	if opts.WriteConcern.Journal == nil || !*opts.WriteConcern.Journal {
		t.Fatalf("write concern = %#v, want journaling enabled", opts.WriteConcern)
	}
}

// TestClientOptionsRespectURITopologyMode verifies direct connections remain an
// explicit deployment choice instead of being forced by the application.
func TestClientOptionsRespectURITopologyMode(t *testing.T) {
	discovery := clientOptions("mongodb://localhost:27017")
	if discovery.Direct != nil && *discovery.Direct {
		t.Fatal("client options force direct mode instead of allowing replica-set discovery")
	}

	direct := clientOptions("mongodb://localhost:27017/?directConnection=true")
	if direct.Direct == nil || !*direct.Direct {
		t.Fatal("client options did not preserve explicit directConnection=true")
	}
}

// TestTopologyTransactionSupport verifies only replica sets and mongos routers
// pass the startup transaction-capability check.
func TestTopologyTransactionSupport(t *testing.T) {
	tests := []struct {
		name     string
		topology topologyInfo
		want     bool
	}{
		{name: "replica set", topology: topologyInfo{SetName: "rs0"}, want: true},
		{name: "sharded cluster", topology: topologyInfo{Message: "isdbgrid"}, want: true},
		{name: "standalone", topology: topologyInfo{}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.topology.supportsTransactions(); got != test.want {
				t.Fatalf("supportsTransactions() = %t, want %t", got, test.want)
			}
		})
	}
}
