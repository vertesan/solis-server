package game

import (
	"fmt"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/server/depot"
	"github.com/vertesan/solis-server/internal/utils"

	"github.com/gofiber/fiber/v3"
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

	value, err := utils.RandomHexString(16)
	if err != nil {
		return fiber.NewError(500, fmt.Sprintf("generate qseed value: %v", err))
	}
	qseed, err := opsolis.FindOrInsertQseed(c, &smodel.Qseed{
		Device: deviceId,
		Qseed:  value,
	})
	if err != nil {
		return fiber.NewError(500, fmt.Sprintf("find or create qseed: %v", err))
	}

	traceId, err := utils.RandomHexString(32)
	if err != nil {
		return fiber.NewError(500, fmt.Sprintf("generate cloud trace ID: %v", err))
	}
	c.Append("X-Cloud-Trace-Context", traceId)
	return c.SendString(qseed.Qseed)
}
