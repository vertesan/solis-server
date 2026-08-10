package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/vertesan/solis-server/internal/db"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/server/depot"
	"github.com/vertesan/solis-server/internal/server/http"
	"github.com/vertesan/solis-server/internal/server/rpc"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func StartServers(base *zap.Logger) (retErr error) {
	// init database connection
	err := db.Initialize()
	if err != nil {
		return fmt.Errorf("failed initializing db: %w", err)
	}
	defer func() {
		disconnectErr := db.Disconnect()
		if disconnectErr == nil {
			zap.L().Info("disconnected from database")
		}
		retErr = errors.Join(retErr, disconnectErr)
	}()

	// get server profile
	depot.ServerProfile, err = opsolis.FindServerProfile(context.Background())
	if err != nil {
		return fmt.Errorf("failed retrieving serverProfile from db: %w", err)
	}

	// spawn child loggers
	grpcLogger := base.Named("GRPC").With(zap.String("layer", "grpc")).Sugar()
	httpLogger := base.Named("HTTP").With(zap.String("layer", "http")).Sugar()

	// prepare for signal context
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	group, ctx := errgroup.WithContext(signalCtx)

	// start servers in goroutine
	group.Go(func() error {
		if err := rpc.StartGRPC(ctx, grpcLogger); err != nil {
			return fmt.Errorf("gRPC server failed: %w", err)
		}
		grpcLogger.Info("grpc server has been shutted down")
		return nil
	})
	group.Go(func() error {
		if err := http.StartHTTP(ctx, httpLogger); err != nil {
			return fmt.Errorf("HTTP server failed: %w", err)
		}
		httpLogger.Info("http server has been shutted down")
		return nil
	})

	return group.Wait()
}
