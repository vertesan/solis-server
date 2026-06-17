package network

import (
	"context"
	"vertesan/solis-server/db"
	"vertesan/solis-server/db/store/opsolis"
	"vertesan/solis-server/network/http"
	"vertesan/solis-server/network/internal/depot"
	"vertesan/solis-server/network/rpc"
	"vertesan/solis-server/utils/rich"
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
