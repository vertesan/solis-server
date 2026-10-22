package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/gofiber/fiber/v3"
	"github.com/vertesan/solis-server/internal/makino/masterdata"
)

func ImportMaster(c fiber.Ctx) error {
	// Files and records stay on the shared filesystem; this endpoint accepts
	// only a small manifest, never paths, credentials, SQL, or binary payloads.
	if len(c.Body()) > 1<<20 {
		return fiber.NewError(fiber.StatusRequestEntityTooLarge, "master publication request exceeds 1 MiB")
	}
	var request masterdata.ImportRequest
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid master publication request")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fiber.NewError(fiber.StatusBadRequest, "unexpected trailing JSON")
	}
	job, err := masterdata.Default.Submit(c, request)
	if errors.Is(err, masterdata.ErrPublicationBusy) || errors.Is(err, masterdata.ErrStaleMaster) {
		return fiber.NewError(fiber.StatusConflict, err.Error())
	}
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(job)
}
