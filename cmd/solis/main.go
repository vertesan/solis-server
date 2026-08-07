package main

import (
	"net/http"
	_ "net/http/pprof"

	_ "github.com/joho/godotenv/autoload"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/vertesan/solis-server/internal/server"
	"github.com/vertesan/solis-server/internal/utils/config"
)

func initLogger() *zap.Logger {
	var zapConfig zap.Config
	if config.GetLogType() == "1" {
		zapConfig = zap.NewProductionConfig()
	} else {
		zapConfig = zap.NewDevelopmentConfig()
	}
	// config.DisableCaller = true
	zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	base := zap.Must(zapConfig.Build(zap.AddStacktrace(zap.DPanicLevel)))
	zap.ReplaceGlobals(base.Named("GLOBAL").With(zap.String("layer", "global")))
	return base
}

func main() {
	baseLogger := initLogger()
	// profiling performance for future improvement in dev environment
	if config.GetEnv() == "dev" {
		go http.ListenAndServe(":6060", nil)
	}

	// start servers
	if err := server.StartServers(baseLogger); err != nil {
		zap.L().Fatal("server stopped with an error", zap.Error(err))
	}

	// flush logger before existing
	zap.L().Sync()
}
