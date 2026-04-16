package servers

import (
	"context"
	"time"
	"vertesan/solis-server/db/crud/opuser"
	pb "vertesan/solis-server/proto/papi"
	"vertesan/solis-server/proto/ptransaction"
	"vertesan/solis-server/utils/jwto"
	"vertesan/solis-server/utils/rich"

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
	userAuth, err := opuser.GetUserAuthByPublicId(context.Background(), req.UserId)
	if err != nil {
		rich.Error("failed to get userAuth: %v", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	if userAuth.Password != req.Password {
		return nil, status.Error(codes.Unauthenticated, "invalid password")
	}

	// retrieve SUID from metadata
	// sub, err := internal.GetSuid(ctx)
	// if err != nil {
	// 	rich.Error("failed retrieving sub from auth token: %v", err)
	// 	return nil, status.Error(codes.InvalidArgument, "invalid argument")
	// }

	// retrieve user from db
	user, err := opuser.FindUser(context.Background(), userAuth.ServerUserId)
	if err != nil {
		rich.Error("failed to FindUser: %v", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	// create one-time token
	oneTimeToken := uuid.New().String()
	err = opuser.UpdateOneTimeToken(context.Background(), userAuth.ServerUserId, oneTimeToken)
	if err != nil {
		rich.Error("failed to UpdateOneTimeToken: %v", err)
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
	userAuth, err := opuser.GetUserAuthByOneTimeToken(context.Background(), req.OnetimeToken)
	if err != nil {
		rich.Error("failed to GetUserAuthByOneTimeToken: %v", err)
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
		rich.Error("failed to create migration token: %v", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &pb.MigrationExecuteResponse{
		Token: tokenStr,
	}, nil
}
