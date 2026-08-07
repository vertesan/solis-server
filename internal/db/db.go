package db

import (
	"context"
	"fmt"
	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/internal/migration"
	"github.com/vertesan/solis-server/internal/db/internal/update"
	"github.com/vertesan/solis-server/internal/utils/config"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

func Initialize() error {
	if err := Connect(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := migration.MigrationCheck(); err != nil {
		return fmt.Errorf("failed to run MigrationCheck: %w", err)
	}
	if err := update.DataUpdateCheck(); err != nil {
		return fmt.Errorf("failed to run DataUpdateCheck: %w", err)
	}

	return nil
}

func Connect() error {
	var err error
	dbUri, err := config.GetDBString()
	if err != nil {
		return err
	}
	dbCensoredUri := regexp.MustCompile(`:[^:]+?@`).ReplaceAllString(dbUri, ":****@")
	zap.L().Sugar().Infof("connecting to database %s", dbCensoredUri)

	depot.Client, err = mongo.Connect(
		options.Client().ApplyURI(dbUri).
			SetBSONOptions(&options.BSONOptions{
				UseJSONStructTags: true,
				NilSliceAsEmpty:   true,
			}).
			SetTimeout(time.Second * 60).
			SetConnectTimeout(time.Second * 30).
			SetMaxConnIdleTime(time.Minute * 5).
			SetMaxConnecting(5).
			SetDirect(true),
	)
	if err != nil {
		return err
	}
	depot.Solis = depot.Client.Database("solis")
	depot.Master = depot.Client.Database("master")
	depot.User = depot.Client.Database("user")

	zap.L().Info("database connection established")
	return nil
}

func Disconnect() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err := depot.Client.Disconnect(ctx); err != nil {
		return fmt.Errorf("failed disconnecting from database: %w", err)
	}
	return nil
}
