package game

import (
	"context"
	"fmt"
	"path"
	"vertesan/solis-server/db/store/opsolis"
	"vertesan/solis-server/utils/config"
	"vertesan/solis-server/utils/rich"

	"github.com/gofiber/fiber/v3"
)

var GetAsset fiber.Handler = func(c fiber.Ctx) error {
	revision := c.Params("revision")
	objectType := c.Params("type")
	objectName := c.Params("object")

	object, err := opsolis.FindManifestObject(context.Background(), objectName, objectType)
	if err != nil {
		rich.Error("failed retrieving octo asset data: %v", err)
		return fiber.NewError(404, "not found")
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
		rich.Error("failed sending resource file %q - %q: %v", objectType, objectName, err)
		return fiber.NewError(500, "internal error")
	}
	return err
}
