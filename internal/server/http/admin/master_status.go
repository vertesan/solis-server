package admin

import (
	"errors"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func MasterImportStatus(c fiber.Ctx) error {
	id := c.Query("id")
	if parsed, err := uuid.Parse(id); err != nil || parsed.String() != id {
		return fiber.NewError(fiber.StatusBadRequest, "invalid import job ID")
	}
	job, err := opsolis.FindMasterImportJob(c, id)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return fiber.NewError(fiber.StatusNotFound, "master import job not found")
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "cannot read master import job")
	}
	return c.JSON(job)
}
