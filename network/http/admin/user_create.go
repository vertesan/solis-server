package admin

import (
	"regexp"
	"strings"
	"vertesan/solis-server/makino/user"
	"vertesan/solis-server/utils"
	"vertesan/solis-server/utils/rich"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type UserCreateRequest struct {
	PublicUserId string `json:"publicUserId,omitempty"`
	ServerUserId string `json:"serverUserId,omitempty"`
	Password     string `json:"password,omitempty"`
}

type UserCreateResponse struct {
	PublicUserId string `json:"publicUserId,omitempty"`
	ServerUserId string `json:"serverUserId,omitempty"`
	Password     string `json:"password,omitempty"`
}

var CreateUser fiber.Handler = func(c fiber.Ctx) error {
	request := &UserCreateRequest{}
	if err := json.Unmarshal(c.Body(), request); err != nil {
		return fiber.NewError(400, "bad request body")
	}
	var publicUserId string
	if request.PublicUserId != "" {
		publicUserId = request.PublicUserId
	} else {
		publicUserId = strings.ToUpper(utils.RandomString(8))
	}
	var serverUserId string
	if request.ServerUserId != "" {
		serverUserId = request.ServerUserId
	} else {
		serverUserId = uuid.NewString()
	}
	var password string
	if request.Password == "" {
		password = utils.RandomString(10)
	} else {
		if !regexp.MustCompile(`^[a-zA-Z0-9]{8,16}$`).Match([]byte(request.Password)) {
			return fiber.NewError(400, "password must be alphanumeric characters within the length of 8 - 16")
		}
	}

	err := user.CreateUser(publicUserId, serverUserId, password)
	if err != nil {
		rich.Error("failed to create user: %v", err)
		return fiber.NewError(500, err.Error())
	}

	return c.JSON(&UserCreateResponse{
		PublicUserId: publicUserId,
		ServerUserId: serverUserId,
		Password:     password,
	})
}
