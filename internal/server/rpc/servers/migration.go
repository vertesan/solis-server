package servers

import (
	"context"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/jwto"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MigrationServer struct {
	pb.UnimplementedMigrationServer
}

func (s *MigrationServer) MigratePassword(ctx context.Context, req *pb.MigrationMigratePasswordRequest) (*pb.MigrationMigrateResponse, error) {
	// retireve userAuth from db
	userAuth, err := opuser.GetUserAuthByPublicId(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get userAuth: %v", err)
	}
	if userAuth.Password != req.Password {
		return nil, status.Error(codes.Unauthenticated, "invalid password")
	}

	// retrieve SUID from metadata
	// sub, err := internal.GetSuid(ctx)
	// if err != nil {
	// 	return nil, status.Errorf(codes.InvalidArgument, "failed retrieving sub from auth token: %v", err)
	// }

	// retrieve user from db
	user, err := opuser.FindUser(ctx, userAuth.ServerUserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to FindUser: %v", err)
	}

	// create one-time token
	oneTimeToken := uuid.New().String()
	err = opuser.UpdateUserAuthOneTimeToken(ctx, userAuth.ServerUserId, oneTimeToken)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to UpdateOneTimeToken: %v", err)
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
	userAuth, err := opuser.GetUserAuthByOneTimeToken(ctx, req.OnetimeToken)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to GetUserAuthByOneTimeToken: %v", err)
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
		return nil, status.Errorf(codes.Internal, "failed to create migration token: %v", err)
	}
	return &pb.MigrationExecuteResponse{
		Token: tokenStr,
	}, nil
}
