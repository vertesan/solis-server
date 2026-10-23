package game

import (
	"fmt"
	"path"

	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/utils/config"

	"github.com/gofiber/fiber/v3"
)

var GetAsset fiber.Handler = func(c fiber.Ctx) error {
	revision := c.Params("revision")
	objectType := c.Params("type")
	objectName := c.Params("object")

	object, err := opsolis.FindManifestObject(c, objectName, objectType)
	if err != nil {
		return fiber.NewError(404, fmt.Sprintf("retrieve manifest asset %q of type %q: %v", objectName, objectType, err))
	}

	// set response contents
	c.Append("Access-Control-Allow-Origin", "*")
	c.Append("Access-Control-Expose-Headers", "X-Octo-Key")
	c.Append("Cache-Control", "public, max-age=3600")
	c.Append("ETag", fmt.Sprintf("\"%s\"", object.Md5))
	err = c.SendFile(
		path.Join(config.GetAssetDir(), revision, objectType, objectName),
		fiber.SendFile{
			Compress:      false,
			CacheDuration: -1,
			MaxAge:        3600,
			Download:      false,
		},
	)
	if err != nil {
		return fiber.NewError(500, fmt.Sprintf("send resource file %q of type %q: %v", objectName, objectType, err))
	}
	return err
}
