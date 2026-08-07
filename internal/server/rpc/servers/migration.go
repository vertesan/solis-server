package servers

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/jwto"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MigrationServer struct {
	pb.UnimplementedMigrationServer
}

func (s *MigrationServer) MigratePassword(ctx context.Context, req *pb.MigrationMigratePasswordRequest) (*pb.MigrationMigrateResponse, error) {
	logger := mcontext.GetLogger(ctx)
	// retireve userAuth from db
	userAuth, err := opuser.GetUserAuthByPublicId(ctx, req.UserId)
	if err != nil {
		logger.Error("failed to get userAuth", zap.Error(err))
		return nil, status.Error(codes.Internal, "internal error")
	}
	if userAuth.Password != req.Password {
		return nil, status.Error(codes.Unauthenticated, "invalid password")
	}

	// retrieve SUID from metadata
	// sub, err := internal.GetSuid(ctx)
	// if err != nil {
	// 	logger.Error("failed retrieving sub from auth token", zap.Error(err))
	// 	return nil, status.Error(codes.InvalidArgument, "invalid argument")
	// }

	// retrieve user from db
	user, err := opuser.FindUser(ctx, userAuth.ServerUserId)
	if err != nil {
		logger.Error("failed to FindUser", zap.Error(err))
		return nil, status.Error(codes.Internal, "internal error")
	}

	// create one-time token
	oneTimeToken := uuid.New().String()
	err = opuser.UpdateUserAuthOneTimeToken(ctx, userAuth.ServerUserId, oneTimeToken)
	if err != nil {
		logger.Error("failed to UpdateOneTimeToken", zap.Error(err))
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &pb.MigrationMigrateResponse{
		LinkedUserId: userAuth.PublicUserId,
		LinkedUser: &ptransaction.User{
			Name:         user.Name,
			ManagerLevel: user.ManagerLevel,
		},
		OnetimeToken: oneTimeToken,
	}, nil
}

func (s *MigrationServer) Execute(ctx context.Context, req *pb.MigrationExecuteRequest) (*pb.MigrationExecuteResponse, error) {
	logger := mcontext.GetLogger(ctx)
	userAuth, err := opuser.GetUserAuthByOneTimeToken(ctx, req.OnetimeToken)
	if err != nil {
		logger.Error("failed to GetUserAuthByOneTimeToken", zap.Error(err))
		return nil, status.Error(codes.Internal, "internal error")
	}

	// create token
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": "firebase-adminsdk-hvn46@solis-firebase.iam.gserviceaccount.com",
		"aud": "https://identitytoolkit.googleapis.com/google.identity.identitytoolkit.v1.IdentityToolkit",
		"sub": "firebase-adminsdk-hvn46@solis-firebase.iam.gserviceaccount.com",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
		"uid": userAuth.FirebaseId,
	})
	tokenStr, err := jwto.CreateToken(token)
	if err != nil {
		logger.Error("failed to create migration token", zap.Error(err))
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &pb.MigrationExecuteResponse{
		Token: tokenStr,
	}, nil
}
