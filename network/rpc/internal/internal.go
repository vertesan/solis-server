package internal

import (
	"context"
	"errors"
	"strings"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

type AuthTokenPayload struct {
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
	Itn string `json:"itn"`
}

func GetSuid(ctx context.Context) (*uuid.UUID, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errors.New("missing metadata")
	}
	tokens := md.Get("x-auth-token")
	if len(tokens) == 0 {
		return nil, errors.New("missing x-auth-token")
	}
	token := tokens[0]
	if token == "" {
		return nil, errors.New("missing x-auth-token")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}
	_, payload, _ := parts[0], parts[1], parts[2]

	var authTokenPayload AuthTokenPayload
	err := json.Unmarshal([]byte(payload), &authTokenPayload)
	if err != nil {
		return nil, err
	}
	if authTokenPayload.Sub == "" {
		return nil, errors.New("missing sub")
	}
	sub, err := uuid.Parse(authTokenPayload.Sub)
	if err != nil {
		return nil, err
	}
	return &sub, nil
}
