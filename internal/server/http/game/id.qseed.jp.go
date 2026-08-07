package game

import (
	"errors"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/server/depot"
	"github.com/vertesan/solis-server/internal/utils"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.uber.org/zap"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var QseedGetId fiber.Handler = func(c fiber.Ctx) error {
	logger := mcontext.GetLogger(c)
	// check header
	appId := c.Get("X-App")
	if appId == "" {
		return fiber.NewError(400, "missing x-app header")
	}
	if appId != depot.ServerProfile.PackageName {
		return fiber.NewError(400, "invalid x-app header")
	}

	deviceId := c.Get("X-Device")
	if deviceId == "" {
		return fiber.NewError(400, "missing x-device header")
	}

	qseed, err := opsolis.FindQseed(c, deviceId)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			logger.Error("failed to get qseed", zap.Error(err))
			return fiber.NewError(500, "internal error")
		}
	}

	if qseed == nil {
		// if not exists, create a new one
		value, err := utils.RandomHexString(16)
		if err != nil {
			logger.Error("failed to create qseed value", zap.Error(err))
			return fiber.NewError(500, "internal error")
		}
		qseed = &smodel.Qseed{
			Device: deviceId,
			Qseed:  value,
		}
		if err := opsolis.UpsertQseed(c, qseed); err != nil {
			logger.Error("failed to create qseed", zap.Error(err))
			return fiber.NewError(500, "internal error")
		}
	}

	traceId, err := utils.RandomHexString(32)
	if err != nil {
		logger.Error("failed to create cloud trace id", zap.Error(err))
		return fiber.NewError(500, "internal error")
	}
	c.Append("X-Cloud-Trace-Context", traceId)
	return c.SendString(qseed.Qseed)
}
