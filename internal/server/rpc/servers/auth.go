package servers

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils"
	"github.com/vertesan/solis-server/internal/utils/jwto"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AuthServer struct {
	pb.UnimplementedAuthServer
}

func (s *AuthServer) Login(ctx context.Context, req *pb.AuthLoginRequest) (*pb.AuthLoginResponse, error) {
	logger := mcontext.GetLogger(ctx)
	_, claim, err := jwto.ParseRS256Token(req.GetFirebaseIDToken())
	if err != nil {
		logger.Error("failed parsing token", zap.Error(err))
		return nil, status.Errorf(codes.InvalidArgument, "bad token")
	}

	// obtain claims
	firebaseIdObj, ok := (*claim)["user_id"]
	if !ok {
		logger.Error("missing user_id in payload")
		return nil, status.Error(codes.InvalidArgument, "bad token")
	}
	firebaseId, ok := firebaseIdObj.(string)
	if !ok {
		logger.Error("non-string user_id in payload")
		return nil, status.Error(codes.InvalidArgument, "bad token")
	}
	requireReAuth := false
	_, ok = (*claim)["puid"]
	if !ok {
		requireReAuth = true
	}

	// retireve userAuth from db
	userAuth, err := opuser.GetUserAuthByFirebaseId(ctx, firebaseId)
	if err != nil {
		logger.Error("failed to GetUserAuthByFirebaseId", zap.Error(err))
		return nil, status.Error(codes.Internal, "internal error")
	}

	// issue a game auth token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userAuth.ServerUserId,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour * 24 * 3).Unix(),
		"itn": utils.RandomString(8),
	})
	gameAuthToken, err := jwto.CreateGameAuthToken(token)
	if err != nil {
		logger.Error("failed to create game auth token", zap.Error(err))
		return nil, status.Error(codes.Internal, "internal error")
	}

	if err := opuser.UpdateUserAuthLastLoginTime(ctx, userAuth.ServerUserId, time.Now().UnixMilli()); err != nil {
		logger.Error("failed to update last login time during user login", zap.Error(err))
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &pb.AuthLoginResponse{
		GameAuthToken: gameAuthToken,
		UserPublic: &ptransaction.UserPublic{
			ServerUserId: userAuth.ServerUserId,
			PublicUserId: userAuth.PublicUserId,
		},
		RequiredFirebaseReauthenticate: requireReAuth,
	}, nil
}
