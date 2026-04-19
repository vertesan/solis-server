package metadata

import (
	"context"
	"errors"
	"vertesan/solis-server/utils/jwto"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

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

	_, claim, err := jwto.ParseHS256Token(token)
	if err != nil {
		return nil, err
	}

	subStr, err := claim.GetSubject()
	if err != nil {
		return nil, err
	}

	sub, err := uuid.Parse(subStr)
	if err != nil {
		return nil, err
	}
	return &sub, nil
}
