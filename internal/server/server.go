package server

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/vertesan/solis-server/internal/db"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/server/depot"
	"github.com/vertesan/solis-server/internal/server/http"
	"github.com/vertesan/solis-server/internal/server/rpc"
	"go.uber.org/zap"
)

func StartServers(base *zap.Logger) {
	// init database
	err := db.Initialize()
	if err != nil {
		zap.L().Sugar().Fatalf("failed initializing db: %v", err)
	}

	// get server profile
	depot.ServerProfile, err = opsolis.FindServerProfile(context.Background())
	if err != nil {
		zap.L().Sugar().Fatalf("failed retrieving serverProfile from db: %v", err)
	}

	// spawn child loggers
	grpcLogger := base.Named("GRPC").With(zap.String("layer", "grpc"))
	httpLogger := base.Named("HTTP").With(zap.String("layer", "http"))

	// prepare for signal context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var servers sync.WaitGroup
	servers.Add(2)
	// start servers in goroutine
	go func() {
		defer servers.Done()
		rpc.StartGRPC(ctx, grpcLogger)
		grpcLogger.Info("grpc server has been shutted down")
	}()
	go func() {
		defer servers.Done()
		http.StartHTTP(ctx, httpLogger)
		httpLogger.Info("http server has been shutted down")
	}()

	// waiting for shutdown signal
	<-ctx.Done()
	servers.Wait()

	db.Disconnect()
	zap.L().Info("disconnected from database")
}
