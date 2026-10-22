package migration

import (
	"context"
	"errors"
	"fmt"
	"time"

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
		zap.L().Info("database initialization completed")
	}

	// Existing level-1 installations receive only missing managed catalogs,
	// never a replay of the initial shop/notice data or an additional level.
	seedCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err = ensureManagedCatalogs(seedCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("failed initializing managed catalogs: %w", err)
	}

	// Index definitions evolve independently from one-time seed data. Ensure
	// them on every startup so existing installations receive new user-state
	// identity constraints without replaying destructive initialization.
	ctx := context.Background()
	if err := ensureUserAuthIndexes(ctx); err != nil {
		return err
	}
	if err := ensureUserIndexes(ctx); err != nil {
		return fmt.Errorf("failed to ensure user indexes: %w", err)
	}

	return nil
}
