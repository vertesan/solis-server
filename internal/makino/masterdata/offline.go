package masterdata

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opsolis"
)

// Recover resolves a previous process's interrupted publication while the
// caller owns the resource directory. MongoDB must already be connected.
func (owner *Ownership) Recover(ctx context.Context) error {
	return owner.withLock(func() error {
		return (&Coordinator{}).Recover(ctx)
	})
}

// Publish executes the same durable publication protocol as the running
// server, synchronously. The ownership lock prevents any server from starting
// until the transaction, outcome resolution, and cleanup have finished.
// A returned job may already be committed even when cleanup returns an error.
func (owner *Ownership) Publish(ctx context.Context, request ImportRequest) (job *ImportJob, err error) {
	err = owner.withLock(func() error {
		if err := validateRequest(request); err != nil {
			return err
		}
		job = newImportJob(request)
		if err := opsolis.InsertMasterImportJob(ctx, job); err != nil {
			return err
		}
		return (&Coordinator{}).run(ctx, job)
	})
	return job, err
}
