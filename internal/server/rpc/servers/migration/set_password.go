package migration

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *MigrationServer) SetPassword(ctx context.Context, req *papi.MigrationSetPasswordRequest) (*papi.MigrationSetPasswordResponse, error) {
	if req.GetPassword() == "" {
		return nil, status.Error(codes.InvalidArgument, "password must not be empty")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.MigrationSetPasswordResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := opuser.UpdateUserAuthPassword(ctx, suid, req.GetPassword()); err != nil {
			return nil, err
		}
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		response = &papi.MigrationSetPasswordResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{User: user})}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}
