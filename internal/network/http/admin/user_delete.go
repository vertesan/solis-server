package admin

import (
	"context"
	"errors"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/user"
	"github.com/vertesan/solis-server/internal/utils/rich"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type UserDeleteRequest struct {
	ServerUserId string `json:"serverUserId,omitempty"`
}

var DeleteUser fiber.Handler = func(c fiber.Ctx) error {
	request := &UserDeleteRequest{}
	if err := json.Unmarshal(c.Body(), request); err != nil {
		return fiber.NewError(400, "bad request body")
	}

	userAuth, err := opuser.GetUserAuthByServerId(context.Background(), request.ServerUserId)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fiber.NewError(400, "user not found")
		}
		rich.Error("failed to GetUserAuthByPublicId: %v", err)
		return fiber.NewError(500, "internal error")
	}

	if err := user.DeleteUser(userAuth.ServerUserId); err != nil {
		rich.Error("failed to delete user: %v", err)
		return fiber.NewError(500, "internal error")
	}

	return c.JSON(&CommonAdminResponse{
		ok:  true,
		msg: "",
	})
}
