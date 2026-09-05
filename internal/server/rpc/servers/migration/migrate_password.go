package migration

import (
	"context"
	"crypto/subtle"
	"errors"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *MigrationServer) MigratePassword(ctx context.Context, req *papi.MigrationMigratePasswordRequest) (*papi.MigrationMigrateResponse, error) {
	if req.GetUserId() == "" || req.GetPassword() == "" {
		return nil, status.Error(codes.Unauthenticated, "invalid user ID or password")
	}
	var response *papi.MigrationMigrateResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		userAuth, err := opuser.GetUserAuthByPublicId(ctx, req.GetUserId())
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.Unauthenticated, "invalid user ID or password")
			}
			return nil, err
		}
		if subtle.ConstantTimeCompare([]byte(userAuth.Password), []byte(req.GetPassword())) != 1 {
			return nil, status.Error(codes.Unauthenticated, "invalid user ID or password")
		}
		user, err := opuser.FindUser(ctx, userAuth.ServerUserId)
		if err != nil {
			return nil, err
		}
		oneTimeToken := uuid.New().String()
		if err := opuser.UpdateUserAuthOneTimeToken(ctx, userAuth.ServerUserId, oneTimeToken); err != nil {
			return nil, err
		}
		response = &papi.MigrationMigrateResponse{
			LinkedUserId: userAuth.PublicUserId,
			LinkedUser:   &ptransaction.User{Name: user.Name, ManagerLevel: user.ManagerLevel},
			OnetimeToken: oneTimeToken,
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}
