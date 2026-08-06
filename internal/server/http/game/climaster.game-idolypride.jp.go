package game

import (
	"path"

	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.uber.org/zap"

	"github.com/gofiber/fiber/v3"
)

var GetMaster fiber.Handler = func(c fiber.Ctx) error {
	logger := mcontext.GetLogger(c)
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
		logger.Error("failed sending master db file", zap.Error(err))
		return fiber.NewError(500, "internal error")
	}
	return err
}
