package auth

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils"
	"github.com/vertesan/solis-server/internal/utils/jwto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *AuthServer) Login(ctx context.Context, req *papi.AuthLoginRequest) (*papi.AuthLoginResponse, error) {
	_, claim, err := jwto.ParseRS256Token(req.GetFirebaseIDToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed parsing token: %v", err)
	}

	// obtain claims
	firebaseIdObj, ok := (*claim)["user_id"]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "missing user_id in payload")
	}
	firebaseId, ok := firebaseIdObj.(string)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "non-string user_id in payload")
	}
	requireReAuth := false
	_, ok = (*claim)["puid"]
	if !ok {
		requireReAuth = true
	}

	// retireve userAuth from db
	userAuth, err := opuser.GetUserAuthByFirebaseId(ctx, firebaseId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to GetUserAuthByFirebaseId: %v", err)
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
		return nil, status.Errorf(codes.Internal, "failed to create game auth token: %v", err)
	}

	if err := opuser.UpdateUserAuthLastLoginTime(ctx, userAuth.ServerUserId, time.Now().UnixMilli()); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update last login time during user login: %v", err)
	}

	return &papi.AuthLoginResponse{
		GameAuthToken: gameAuthToken,
		UserPublic: &ptransaction.UserPublic{
			ServerUserId: userAuth.ServerUserId,
			PublicUserId: userAuth.PublicUserId,
		},
		RequiredFirebaseReauthenticate: requireReAuth,
	}, nil
}
