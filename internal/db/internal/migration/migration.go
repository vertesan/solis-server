package migration

import (
	"context"
	"errors"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/store/opsolis"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
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

	if migrationFlag < 1 {
		zap.L().Info("looks like this is the first startup, initializing database...")
		if err := level1Migration(); err != nil {
			return fmt.Errorf("failed database migration level 1: %w", err)
		}
		migrationFlag = 1
		zap.L().Info("database initialization completed")
	}

	return nil
}
