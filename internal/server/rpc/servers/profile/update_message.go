package profile

import (
	"context"
	"unicode/utf8"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ProfileServer) UpdateMessage(ctx context.Context, req *papi.ProfileUpdateMessageRequest) (*papi.ProfileUpdateMessageResponse, error) {
	if req == nil || !utf8.ValidString(req.GetMessage()) || utf8.RuneCountInString(req.GetMessage()) > 60 {
		return nil, viber.InvalidArgumentf("message must contain at most 60 characters")
	}
	common, err := updateProfile(ctx, func(_ context.Context, _ string, profile *ptransaction.UserProfile) error {
		profile.Message = req.GetMessage()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &papi.ProfileUpdateMessageResponse{CommonResponse: common}, nil
}
