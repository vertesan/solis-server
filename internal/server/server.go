package server

import (
	"context"
	"github.com/vertesan/solis-server/internal/db"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/server/http"
	"github.com/vertesan/solis-server/internal/server/internal/depot"
	"github.com/vertesan/solis-server/internal/server/rpc"
	"github.com/vertesan/solis-server/internal/utils/rich"
)

func StartServer() {
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

	// start servers
	go rpc.StartGRPC()
	go http.StartHTTP()
	// TODO: graceful shutdown and cleanup
	select {}

	db.Disconnect()
}

func GetAppVersion() string {
	return depot.ServerProfile.AppVersion
}

func GetMasterVersion() string {
	return depot.ServerProfile.MasterVersion
}
