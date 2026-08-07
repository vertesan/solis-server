package interceptor

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/server/depot"
	"github.com/vertesan/solis-server/internal/utils"
	"github.com/vertesan/solis-server/internal/utils/jwto"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.uber.org/zap"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type AuthTokenKey struct{}

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
	errInternal             = status.Errorf(codes.Internal, "internal error")

	noNeedTokenSet = map[string]struct{}{
		"/api.Auth/Login":                {},
		"/api.System/Check":              {},
		"/api.Migration/MigratePassword": {},
		"/api.Migration/Execute":         {},
	}
)

func UnaryServerInterceptor(base *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		logger := base.With(zap.String("api", info.FullMethod))
		logger.Sugar().Infof("[gRPC] %s", info.FullMethod)

		// retrieve metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, errMissingMetadata
		}

		// verify metadata
		if err := verifyMetadata(md, info); err != nil {
			logger.Error("failed verifying metadata", zap.Error(err))
			return nil, err
		}

		// check x-app-request-id
		requestId := md.Get("x-app-request-id")
		if len(requestId) > 0 {
			logger = logger.With(zap.String("request_id", requestId[0]))
			// TODO: dedulplicate request by checking request id to ensure idempotency
		}

		// send metadata back
		header := metadata.New(map[string]string{
			"x-server-time": fmt.Sprintf("%d", utils.GetTimestamp()),
			"cache-control": "no-store",
		})
		if err := grpc.SendHeader(ctx, header); err != nil {
			logger.Error("failed sending header", zap.Error(err))
			return nil, err
		}

		// embed uuid into context
		if _, ok := noNeedTokenSet[info.FullMethod]; !ok {
			suid, err := getSuid(ctx)
			if err != nil {
				logger.Error("failed to get suid", zap.Error(err))
				return nil, err
			}
			logger = logger.With(zap.String("suid", suid.String()))
			ctx = context.WithValue(ctx, AuthTokenKey{}, suid)
		}
		// thread logger to context
		ctx = mcontext.WithLogger(ctx, logger)

		// invoke handler
		m, err := handler(ctx, req)
		if err != nil {
			logger.Error("RPC failed with error", zap.Error(err))
		}
		return m, abstractErrorMsg(err)
	}
}

func abstractErrorMsg(err error) error {
	code := status.Code(err)
	if code == codes.OK {
		return nil
	}
	return status.Error(code, code.String())
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

func getSuid(ctx context.Context) (*uuid.UUID, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errMissingMetadata
	}
	tokens := md.Get("x-auth-token")
	if len(tokens) == 0 {
		return nil, errMissingToken
	}
	token := tokens[0]
	if token == "" {
		return nil, errMissingToken
	}

	_, claim, err := jwto.ParseHS256Token(token)
	if err != nil {
		return nil, errors.Join(errInvalidToken, err)
	}

	subStr, err := claim.GetSubject()
	if err != nil {
		return nil, errors.Join(errInvalidToken, err)
	}

	sub, err := uuid.Parse(subStr)
	if err != nil {
		return nil, errors.Join(errInvalidToken, err)
	}
	return &sub, nil
}
