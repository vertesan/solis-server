package game

import (
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/rich"
	"path"

	"github.com/gofiber/fiber/v3"
)

var GetMaster fiber.Handler = func(c fiber.Ctx) error {
	filename := c.Params("filename")

	// set response contents
	err := c.SendFile(
		path.Join(config.GetDBDir(), filename),
		fiber.SendFile{
			Compress:      false,
			CacheDuration: -1,
			MaxAge:        3600,
			Download:      false,
		},
	)
	if err != nil {
		rich.Error("failed sending master db file: %v", err)
		return fiber.NewError(500, "internal error")
	}
	return err
}
