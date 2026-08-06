package rich

import (
	"go.uber.org/zap"
)

func Info(text string, args ...interface{}) {
	GetLogger().Infof(text, args...)
}

func Warn(text string, args ...interface{}) {
	GetLogger().Warnf(text, args...)
}

func Error(text string, args ...interface{}) {
	GetLogger().Errorf(text, args...)
}

func Panic(text string, args ...interface{}) {
	GetLogger().Panicf(text, args...)
}

func Fatal(text string, args ...interface{}) {
	GetLogger().Fatalf(text, args...)
}

func Sync() error {
	return GetLogger().Sync()
}

func GetLogger() *zap.SugaredLogger {
	return zap.L().Sugar()
}
