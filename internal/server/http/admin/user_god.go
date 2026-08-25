package admin

import (
	"errors"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/makino/user"
	"github.com/vertesan/solis-server/internal/utils/mcontext"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

type UserGodRequest struct {
	PublicUserId string     `json:"publicUserId,omitempty"`
	ServerUserId string     `json:"serverUserId,omitempty"`
	Feature      GodFeature `json:"feature,omitempty"`
}

type GodFeature struct {
	Message string `json:"message,omitempty"`
}

type UserGodResponse struct {
	Ok           bool           `json:"ok"`
	ServerUserId string         `json:"serverUserId"`
	Collections  map[string]int `json:"collections"`
}

var GodUser fiber.Handler = func(c fiber.Ctx) error {
	request := &UserGodRequest{}
	if err := json.Unmarshal(c.Body(), request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "bad request body")
	}
	if (request.PublicUserId == "") == (request.ServerUserId == "") {
		return fiber.NewError(fiber.StatusBadRequest, "exactly one of publicUserId or serverUserId is required")
	}
	if request.Feature.Message == "" {
		request.Feature.Message = umodel.GodMessageFeatureAll
	}
	switch request.Feature.Message {
	case umodel.GodMessageFeatureNone, umodel.GodMessageFeatureAll, umodel.GodMessageFeatureEnjoy:
	default:
		return fiber.NewError(fiber.StatusBadRequest, "feature.message must be one of none, all, or enjoy")
	}

	result, err := user.GodUser(c, request.PublicUserId, request.ServerUserId, user.GodFeature{
		Message: request.Feature.Message,
	})
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fiber.NewError(fiber.StatusBadRequest, "user not found")
		}
		mcontext.GetLogger(c).Error("failed to unlock user god mode", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}
	return c.JSON(&UserGodResponse{
		Ok: true, ServerUserId: result.ServerUserId, Collections: result.Collections,
	})
}
