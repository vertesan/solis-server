package game

import (
	"fmt"
	"path"

	"github.com/vertesan/solis-server/internal/utils/config"

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
		return fiber.NewError(500, fmt.Sprintf("send master database file %q: %v", filename, err))
	}
	return err
}
