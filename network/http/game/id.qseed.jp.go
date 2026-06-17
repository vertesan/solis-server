package game

import (
	"context"
	"errors"
	"vertesan/solis-server/db/def/solis/sstruct"
	"vertesan/solis-server/db/store/opsolis"
	"vertesan/solis-server/network/internal/depot"
	"vertesan/solis-server/utils"
	"vertesan/solis-server/utils/rich"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var QseedGetId fiber.Handler = func(c fiber.Ctx) error {
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

	qseed, err := opsolis.FindQseed(context.Background(), deviceId)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			rich.Error("failed to get qseed: %v", err)
			return fiber.NewError(500, "internal error")
		}
	}

	if qseed == nil {
		// if not exists, create a new one
		qseed = &sstruct.Qseed{
			Device: deviceId,
			Qseed:  utils.RandomHexString(16),
		}
		if err := opsolis.UpsertQseed(context.Background(), qseed); err != nil {
			rich.Error("failed to create qseed: %v", err)
			return fiber.NewError(500, "internal error")
		}
	}

	c.Append("X-Cloud-Trace-Context", utils.RandomHexString(32))
	return c.SendString(qseed.Qseed)
}
