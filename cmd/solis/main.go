package main

import (
	_ "github.com/joho/godotenv/autoload"
	"net/http"
	_ "net/http/pprof"

	"github.com/vertesan/solis-server/internal/network"
	"github.com/vertesan/solis-server/internal/utils/config"
)

func main() {
	// profiling performance for future improvement in dev environment
	if config.GetEnv() == "dev" {
		go http.ListenAndServe(":6060", nil)
	}

	// start server
	network.StartServer()
}
