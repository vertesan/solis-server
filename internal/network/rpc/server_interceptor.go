package rpc

import (
	"context"
	"fmt"
	"github.com/vertesan/solis-server/internal/network/internal/depot"
	"github.com/vertesan/solis-server/internal/utils"
	"github.com/vertesan/solis-server/internal/utils/jwto"
	"github.com/vertesan/solis-server/internal/utils/rich"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	errMissingMetadata      = status.Errorf(codes.InvalidArgument, "missing metadata")
	errMissingAppVersion    = status.Errorf(codes.InvalidArgument, "missing app version")
	errInvalidAppVersion    = status.Errorf(codes.InvalidArgument, "invalid app version")
	errMissingPlatform      = status.Errorf(codes.InvalidArgument, "missing platform")
	errInvalidPlatform      = status.Errorf(codes.InvalidArgument, "invalid platform")
	errMissingToken         = status.Errorf(codes.Unauthenticated, "missing token")
	errInvalidToken         = status.Errorf(codes.Unauthenticated, "invalid token")
	errMissingMasterVersion = status.Errorf(codes.InvalidArgument, "missing master version")
	errInvalidMasterVersion = status.Errorf(codes.InvalidArgument, "invalid master version")

	noNeedTokenSet = map[string]struct{}{
		"/api.Auth/Login":                {},
		"/api.Migration/MigratePassword": {},
		"/api.Migration/Execute":         {},
	}
)

var unaryServerInterceptor grpc.UnaryServerInterceptor = func(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp any, err error) {
	rich.Info("[gRPC] %s", info.FullMethod)

	// retrieve metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errMissingMetadata
	}

	// verify metadata
	if err := verifyMetadata(md, info); err != nil {
		rich.Error("failed verifying metadata: %v", err)
		return nil, err
	}

	// TODO: check x-app-request-id

	// send metadata back
	header := metadata.New(map[string]string{
		"x-server-time": fmt.Sprintf("%d", utils.GetTimestamp()),
		"cache-control": "no-store",
	})
	if err := grpc.SendHeader(ctx, header); err != nil {
		rich.Error("failed sending header: %v", err)
		return nil, err
	}

	// invoke RPC
	m, err := handler(ctx, req)
	if err != nil {
		rich.Error("RPC failed with error: %v", err)
	}
	return m, err
}

func verifyMetadata(md metadata.MD, info *grpc.UnaryServerInfo) error {
	if info.FullMethod == "/api.System/Check" {
		return nil
	}
	// verify x-app-version
	appVersions := md.Get("x-app-version")
	if len(appVersions) == 0 {
		return errMissingAppVersion
	}
	if appVersions[0] != depot.ServerProfile.AppVersion {
		return errInvalidAppVersion
	}

	// verify x-platform
	platform := md.Get("x-platform")
	if len(platform) == 0 {
		return errMissingPlatform
	}
	if platform[0] != depot.ServerProfile.Platform {
		return errInvalidPlatform
	}

	// verify x-seed-id
	// TODO: skip for now

	// verify x-auth-token
	if _, ok := noNeedTokenSet[info.FullMethod]; ok {
		return nil
	}
	if err := verifyToken(md.Get("x-auth-token")); err != nil {
		return err
	}

	// verify x-master-version
	if info.FullMethod == "/api.Master/Get" {
		return nil
	}
	masterVersion := md.Get("x-master-version")
	if len(masterVersion) == 0 {
		return errMissingMasterVersion
	}
	if masterVersion[0] != depot.ServerProfile.MasterVersion {
		return errInvalidMasterVersion
	}

	return nil
}

func verifyToken(token []string) error {
	if len(token) == 0 {
		return errMissingToken
	}
	gameAuthToken := token[0]
	_, _, err := jwto.ParseHS256Token(gameAuthToken)
	if err != nil {
		return err
	}
	return nil
}
