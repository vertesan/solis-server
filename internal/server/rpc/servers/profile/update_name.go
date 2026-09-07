package profile

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ProfileServer) UpdateName(ctx context.Context, req *papi.ProfileUpdateNameRequest) (*papi.ProfileUpdateNameResponse, error) {
	name := req.GetName()
	if !utf8.ValidString(name) || strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 10 {
		return nil, viber.InvalidArgumentf("name must contain 1 to 10 characters")
	}
	common, err := updateProfileUser(ctx, func(_ context.Context, _ string, user *ptransaction.User) error {
		user.Name = name
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &papi.ProfileUpdateNameResponse{CommonResponse: common}, nil
}
