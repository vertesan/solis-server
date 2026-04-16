package test

import (
	"context"
	"fmt"
	"time"
	"vertesan/solis-server/network"
	"vertesan/solis-server/utils"
	"vertesan/solis-server/utils/config"
	"vertesan/solis-server/utils/jwto"

	"github.com/gofiber/fiber/v3/client"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type Environment int

const (
	Dev Environment = iota
	Prd
)

type TestConfig struct {
	Env Environment

	GrpcFqdnDev string
	GrpcFqdnPrd string
	HttpUrlDev  string
	HttpUrlPrd  string

	GrpcFqdn string
	HttpUrl  string

	FirebaseProductKey string
	XAppVersion        string
	XPlatform          string
	TestUserPID        string
	TestUserPass       string

	GrpcConn   *grpc.ClientConn
	HttpClient *client.Client
}

var (
	testConfig = TestConfig{
		Env:                Dev,
		GrpcFqdnDev:        "localhost:50051",
		GrpcFqdnPrd:        "",
		HttpUrlDev:         "http://localhost:50052",
		HttpUrlPrd:         "",
		FirebaseProductKey: "AIzaSyCabjC7_mOU21qkcNobnhN1khTi1-1F-kA",
		XAppVersion:        "4.10.4",
		XPlatform:          "Android",
		TestUserPID:        "00000000",
		TestUserPass:       "passw0rd",
	}
)

func getBasicGrpcCtx() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	ctx = metadata.AppendToOutgoingContext(ctx,
		"x-app-version", testConfig.XAppVersion,
		"x-platform", testConfig.XPlatform,
	)
	return ctx, cancel
}

func getTokenedGrpcCtx() (context.Context, context.CancelFunc, error) {
	ctx, cancel := getBasicGrpcCtx()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": testConfig.TestUserPID,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour * 24 * 3).Unix(),
		"itn": utils.RandomString(8),
	})
	authToken, err := jwto.CreateGameAuthToken(token)
	if err != nil {
		return nil, nil, err
	}
	ctx = metadata.AppendToOutgoingContext(ctx,
		"x-auth-token", authToken,
	)
	return ctx, cancel, nil
}

func getFullGrpcCtx() (context.Context, context.CancelFunc, error) {
	ctx, cancel, err := getTokenedGrpcCtx()
	if err != nil {
		return nil, nil, err
	}
	ctx = metadata.AppendToOutgoingContext(ctx,
		"x-master-version", network.GetMasterVersion(),
	)
	return ctx, cancel, nil
}

func getAdminAuthHeader() map[string]string {
	return map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", config.GetAdminAuth()),
	}
}
