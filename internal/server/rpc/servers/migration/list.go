package migration

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *MigrationServer) List(ctx context.Context, _ *papi.Empty) (*papi.MigrationListResponse, error) {
	auth, err := opuser.GetUserAuthByServerId(ctx, viber.GetSuid(ctx).String())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user auth: %v", err)
	}
	return &papi.MigrationListResponse{
		Google:         auth.FirebaseId != "",
		Password:       auth.Password != "",
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
