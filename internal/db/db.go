package db

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/internal/migration"
	"github.com/vertesan/solis-server/internal/db/internal/update"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/rich"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func Initialize() error {
	if err := Connect(); err != nil {
		rich.Fatal("failed to connect to database: %v", err)
	}

	if err := migration.MigrationCheck(); err != nil {
		rich.Fatal("failed to run MigrationCheck: %v", err)
	}
	if err := update.DataUpdateCheck(); err != nil {
		rich.Fatal("failed to run DataUpdateCheck: %v", err)
	}

	return nil
}

func Connect() error {
	var err error
	dbUri := config.GetDBString()
	dbCensoredUri := regexp.MustCompile(`:[^:]+?@`).ReplaceAllString(dbUri, ":****@")
	rich.Info("connecting to database %s", dbCensoredUri)

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

	rich.Info("database connection established")
	return nil
}

func Disconnect() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	if err := depot.Client.Disconnect(ctx); err != nil {
		rich.Fatal("failed disconnecting to database: %v", err)
	}
}
