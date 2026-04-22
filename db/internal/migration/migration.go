package migration

import (
	"context"
	"errors"
	"fmt"
	"vertesan/solis-server/db/crud/opsolis"
	"vertesan/solis-server/utils/rich"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Check if this is the first startup, and potential future database migration
func MigrationCheck() error {
	var migrationFlag int32
	migration, err := opsolis.FindMigration(context.Background())
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
	} else {
		migrationFlag = migration.Migration
	}

	// first startup
	if migrationFlag < 1 {
		rich.Info("migration flag < 1, looks like this is the first startup, initializing server profile...")
		if err := level1Migration(); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		rich.Info("server profile initialized")
	}

	return nil
}
