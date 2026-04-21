package servers

import (
	"context"
	"time"
	"vertesan/solis-server/db/crud/opuser"
	pb "vertesan/solis-server/proto/papi"
	"vertesan/solis-server/proto/ptransaction"
	"vertesan/solis-server/utils"
	"vertesan/solis-server/utils/jwto"
	"vertesan/solis-server/utils/rich"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AuthServer struct {
	pb.UnimplementedAuthServer
}

func (s *AuthServer) Login(ctx context.Context, req *pb.AuthLoginRequest) (*pb.AuthLoginResponse, error) {
	_, claim, err := jwto.ParseRS256Token(req.GetFirebaseIDToken())
	if err != nil {
		rich.Error("failed parsing token: %v", err)
		return nil, status.Errorf(codes.InvalidArgument, "bad token")
	}

	// obtain claims
	firebaseIdObj, ok := (*claim)["user_id"]
	if !ok {
		rich.Error("missing user_id in payload")
		return nil, status.Error(codes.InvalidArgument, "bad token")
	}
	firebaseId, ok := firebaseIdObj.(string)
	if !ok {
		rich.Error("non-string user_id in payload")
		return nil, status.Error(codes.InvalidArgument, "bad token")
	}
	requireReAuth := false
	_, ok = (*claim)["puid"]
	if !ok {
		requireReAuth = true
	}

	// retireve userAuth from db
	userAuth, err := opuser.GetUserAuthByFirebaseId(context.Background(), firebaseId)
	if err != nil {
		rich.Error("failed to GetUserAuthByFirebaseId: %v", err)
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
		rich.Error("failed to create game auth token: %v", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	if err := opuser.UpdateUserAuthLastLoginTime(context.Background(), userAuth.ServerUserId, time.Now().UnixMilli()); err != nil {
		rich.Error("failed to update last login time during user login: %v", err)
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
