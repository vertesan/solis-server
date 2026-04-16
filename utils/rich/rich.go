package rich

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.SugaredLogger

func init() {
	config := zap.NewDevelopmentConfig()
	config.DisableCaller = true
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	logger = zap.Must(config.Build(zap.AddStacktrace(zap.DPanicLevel))).Sugar()
}

func Info(text string, args ...interface{}) {
	logger.Infof(text, args...)
}

func Warn(text string, args ...interface{}) {
	logger.Warnf(text, args...)
}

func Error(text string, args ...interface{}) {
	logger.Errorf(text, args...)
}

func Panic(text string, args ...interface{}) {
	logger.Panicf(text, args...)
}

func Fatal(text string, args ...interface{}) {
	logger.Fatalf(text, args...)
}

func Sync() error {
	return logger.Sync()
}

func GetLogger() *zap.SugaredLogger {
	return logger
}
