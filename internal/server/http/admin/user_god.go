package admin

import (
	"errors"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/makino/user"
	"github.com/vertesan/solis-server/internal/server/http/internal/diver"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type UserGodRequest struct {
	PublicUserId string     `json:"publicUserId,omitempty"`
	ServerUserId string     `json:"serverUserId,omitempty"`
	Feature      GodFeature `json:"feature,omitempty"`
}

type GodFeature struct {
	Message string `json:"message,omitempty"`
	Fan     string `json:"fan,omitempty"`
	Bond    string `json:"bond,omitempty"`
}

type UserGodResponse struct {
	Ok           bool           `json:"ok"`
	ServerUserId string         `json:"serverUserId"`
	Collections  map[string]int `json:"collections"`
}

var GodUser fiber.Handler = func(c fiber.Ctx) error {
	request := &UserGodRequest{}
	if err := json.Unmarshal(c.Body(), request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("decode user godification request: %s", diver.JSONErrorDescription(err)))
	}
	if (request.PublicUserId == "") == (request.ServerUserId == "") {
		return fiber.NewError(fiber.StatusBadRequest, "exactly one of publicUserId or serverUserId is required")
	}
	var valid bool
	request.Feature.Message, valid = normalizeGodFeature(request.Feature.Message)
	if !valid {
		return fiber.NewError(fiber.StatusBadRequest, "feature.message must be one of none, all, or enjoy")
	}
	request.Feature.Fan, valid = normalizeGodFeature(request.Feature.Fan)
	if !valid {
		return fiber.NewError(fiber.StatusBadRequest, "feature.fan must be one of none, all, or enjoy")
	}
	request.Feature.Bond, valid = normalizeGodFeature(request.Feature.Bond)
	if !valid {
		return fiber.NewError(fiber.StatusBadRequest, "feature.bond must be one of none, all, or enjoy")
	}

	result, err := user.GodUser(c, request.PublicUserId, request.ServerUserId, user.GodFeature{
		Message: request.Feature.Message,
		Fan:     request.Feature.Fan,
		Bond:    request.Feature.Bond,
	})
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fiber.NewError(fiber.StatusBadRequest, "user not found")
		}
		return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("unlock user god mode: %v", err))
	}
	return c.JSON(&UserGodResponse{
		Ok: true, ServerUserId: result.ServerUserId, Collections: result.Collections,
	})
}

func normalizeGodFeature(value string) (string, bool) {
	if value == "" {
		return umodel.GodMessageFeatureAll, true
	}
	switch value {
	case umodel.GodMessageFeatureNone, umodel.GodMessageFeatureAll, umodel.GodMessageFeatureEnjoy:
		return value, true
	default:
		return "", false
	}
}
