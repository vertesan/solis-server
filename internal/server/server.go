package server

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/vertesan/solis-server/internal/db"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/server/http"
	"github.com/vertesan/solis-server/internal/server/depot"
	"github.com/vertesan/solis-server/internal/server/rpc"
	"github.com/vertesan/solis-server/internal/utils/rich"
)

func StartServers() {
	// init database
	err := db.Initialize()
	if err != nil {
		rich.Fatal("failed initializing db: %v", err)
	}

	// get server profile
	depot.ServerProfile, err = opsolis.FindServerProfile(context.Background())
	if err != nil {
		rich.Fatal("failed retrieving serverProfile from db: %v", err)
	}

	// prepare for signal context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var servers sync.WaitGroup
	servers.Add(2)
	// start servers in goroutine
	go func() {
		defer servers.Done()
		rpc.StartGRPC(ctx)
		rich.Info("grpc server has been shutted down")
	}()
	go func() {
		defer servers.Done()
		http.StartHTTP(ctx)
		rich.Info("http server has been shutted down")
	}()

	// waiting for shutdown signal
	<-ctx.Done()
	servers.Wait()

	db.Disconnect()
	rich.Info("disconnected from database")
}
