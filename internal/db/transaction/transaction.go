package transaction

import (
	"context"
	"fmt"
	"github.com/vertesan/solis-server/internal/db/internal/depot"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

func WithTransaction(
	fn func(ctx context.Context) (any, error),
	rConcern *readconcern.ReadConcern,
	wConcern *writeconcern.WriteConcern,
) error {
	// prepare read/wirte concerns
	if rConcern == nil {
		rConcern = readconcern.Snapshot()
	}
	if wConcern == nil {
		wConcern = writeconcern.Majority()
	}
	txnOpts := options.Transaction().
		SetReadConcern(rConcern).
		SetWriteConcern(wConcern)

	// start session
	session, err := depot.Client.StartSession()
	if err != nil {
		return fmt.Errorf("failed start session: %w", err)
	}
	defer session.EndSession(context.Background())

	// execute transaction within session
	_, err = session.WithTransaction(
		context.Background(),
		fn,
		txnOpts,
	)
	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}
	return nil
}
