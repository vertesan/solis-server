package db

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/internal/migration"
	"github.com/vertesan/solis-server/internal/db/internal/update"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"go.uber.org/zap"
)

// Initialize connects to MongoDB, repairs legacy singleton documents, runs
// migrations and data updates, and then installs the required indexes.
func Initialize() (retErr error) {
	if retErr = Connect(); retErr != nil {
		return fmt.Errorf("failed to connect to database: %w", retErr)
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, Disconnect())
		}
	}()

	if retErr = migration.MigrationCheck(); retErr != nil {
		return fmt.Errorf("failed to run MigrationCheck: %w", retErr)
	}
	if retErr = update.DataUpdateCheck(); retErr != nil {
		return fmt.Errorf("failed to run DataUpdateCheck: %w", retErr)
	}

	return nil
}

func Connect() error {
	dbUri, err := config.GetDBString()
	if err != nil {
		return err
	}
	dbCensoredUri := regexp.MustCompile(`:[^:]+?@`).ReplaceAllString(dbUri, ":****@")
	zap.L().Sugar().Infof("connecting to database %s", dbCensoredUri)

	client, err := mongo.Connect(clientOptions(dbUri))
	if err != nil {
		return fmt.Errorf("failed to create MongoDB client: %w", err)
	}
	connectCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Ping(connectCtx, nil); err != nil {
		return errors.Join(fmt.Errorf("failed to ping MongoDB: %w", err), disconnectClient(client))
	}
	var hello topologyInfo
	if err := client.Database("admin").RunCommand(connectCtx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return errors.Join(fmt.Errorf("failed to inspect MongoDB topology: %w", err), disconnectClient(client))
	}
	if !hello.supportsTransactions() {
		return errors.Join(errors.New("MongoDB deployment must be a replica set or sharded cluster to support transactions"), disconnectClient(client))
	}

	depot.Client = client
	depot.Solis = client.Database("solis")
	depot.Master = client.Database("master")
	depot.User = client.Database("user")

	zap.L().Info("database connection established")
	return nil
}

type topologyInfo struct {
	SetName string `bson:"setName"`
	Message string `bson:"msg"`
}

// supportsTransactions reports whether the hello response identifies a
// replica set member or a mongos router.
func (topology topologyInfo) supportsTransactions() bool {
	return topology.SetName != "" || topology.Message == "isdbgrid"
}

// clientOptions applies the service's consistency and durability requirements
// on top of the supplied MongoDB URI.
func clientOptions(dbUri string) *options.ClientOptions {
	journaled := true
	wConcern := writeconcern.Majority()
	wConcern.Journal = &journaled
	return options.Client().ApplyURI(dbUri).
		SetBSONOptions(&options.BSONOptions{
			UseJSONStructTags: true,
			NilSliceAsEmpty:   true,
		}).
		SetTimeout(time.Second * 60).
		SetConnectTimeout(time.Second * 30).
		SetMaxConnIdleTime(time.Minute * 5).
		SetMaxConnecting(5).
		SetReadConcern(readconcern.Majority()).
		SetReadPreference(readpref.Primary()).
		SetWriteConcern(wConcern)
}

func Disconnect() error {
	if depot.Client == nil {
		return nil
	}
	client := depot.Client
	depot.Client = nil
	depot.Solis = nil
	depot.Master = nil
	depot.User = nil
	if err := disconnectClient(client); err != nil {
		return fmt.Errorf("failed disconnecting from database: %w", err)
	}
	return nil
}

// disconnectClient closes a MongoDB client with an independent bounded context
// so cleanup still runs when the caller's context has expired.
func disconnectClient(client *mongo.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	return client.Disconnect(ctx)
}
