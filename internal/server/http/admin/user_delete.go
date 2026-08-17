package admin

import (
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/user"
	"github.com/vertesan/solis-server/internal/server/http/internal/diver"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.uber.org/zap"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type UserDeleteRequest struct {
	ServerUserId string `json:"serverUserId,omitempty"`
}

var DeleteUser fiber.Handler = func(c fiber.Ctx) error {
	logger := mcontext.GetLogger(c)
	request := &UserDeleteRequest{}
	if err := json.Unmarshal(c.Body(), request); err != nil {
		return fiber.NewError(400, "bad request body")
	}

	userAuth, err := opuser.GetUserAuthByServerId(c, request.ServerUserId)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fiber.NewError(400, "user not found")
		}
		logger.Error("failed to GetUserAuthByPublicId", zap.Error(err))
		return fiber.NewError(500, "internal error")
	}

	if err := user.DeleteUser(c, userAuth.ServerUserId); err != nil {
		logger.Error("failed to delete user", zap.Error(err))
		return fiber.NewError(500, "internal error")
	}

	return c.JSON(&diver.CommonAdminResponse{
		Ok:  true,
		Msg: "",
	})
}
