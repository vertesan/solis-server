package servers

import (
	"context"
	"errors"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"github.com/vertesan/solis-server/internal/utils/jwto"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MigrationServer struct {
	papi.UnimplementedMigrationServer
}

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

func (s *MigrationServer) MigratePassword(ctx context.Context, req *papi.MigrationMigratePasswordRequest) (*papi.MigrationMigrateResponse, error) {
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

	return &papi.MigrationMigrateResponse{
		LinkedUserId: userAuth.PublicUserId,
		LinkedUser: &ptransaction.User{
			Name:         user.Name,
			ManagerLevel: user.ManagerLevel,
		},
		OnetimeToken: oneTimeToken,
	}, nil
}

func (s *MigrationServer) Execute(ctx context.Context, req *papi.MigrationExecuteRequest) (*papi.MigrationExecuteResponse, error) {
	if req.GetOnetimeToken() == "" {
		return nil, status.Error(codes.Unauthenticated, "invalid one-time token")
	}
	userAuth, err := opuser.GetUserAuthByOneTimeToken(ctx, req.GetOnetimeToken())
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, status.Error(codes.Unauthenticated, "invalid one-time token")
		}
		return nil, status.Errorf(codes.Internal, "failed to get one-time token: %v", err)
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
	// Consume only after token signing succeeds. Concurrent executions may both
	// prepare a response, but exactly one can atomically clear the source token.
	if _, err := opuser.ConsumeUserAuthOneTimeToken(ctx, userAuth.ServerUserId, req.GetOnetimeToken()); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, status.Error(codes.Unauthenticated, "invalid one-time token")
		}
		return nil, status.Errorf(codes.Internal, "failed to consume one-time token: %v", err)
	}
	return &papi.MigrationExecuteResponse{
		Token: tokenStr,
	}, nil
}
