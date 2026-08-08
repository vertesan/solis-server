package transaction

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/internal/depot"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

func WithTransaction(
	ctx context.Context,
	fn func(ctx context.Context) (any, error),
) error {
	// MongoDB does not support nested transactions. Reuse the active transaction
	// so helpers can provide atomic behavior both on their own and as part of a
	// larger state transition.
	if session := mongo.SessionFromContext(ctx); session != nil && session.TransactionRunning() {
		_, err := fn(ctx)
		return err
	}

	// start session
	session, err := depot.Client.StartSession()
	if err != nil {
		return fmt.Errorf("failed start session: %w", err)
	}
	defer session.EndSession(ctx)

	// execute transaction within session
	_, err = session.WithTransaction(
		ctx,
		fn,
		transactionOptions(),
	)
	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}
	return nil
}

// transactionOptions defines the snapshot, primary-read, and
// majority-journaled guarantees shared by every application transaction.
func transactionOptions() *options.TransactionOptionsBuilder {
	journaled := true
	wConcern := writeconcern.Majority()
	wConcern.Journal = &journaled
	return options.Transaction().
		SetReadConcern(readconcern.Snapshot()).
		SetReadPreference(readpref.Primary()).
		SetWriteConcern(wConcern)
}
