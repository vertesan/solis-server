package interceptor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/depot"
	"github.com/vertesan/solis-server/internal/utils"
	"github.com/vertesan/solis-server/internal/utils/jwto"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.uber.org/zap"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type AuthTokenKey struct{}

var jstLocation = time.FixedZone("JST", 9*60*60)

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

func UnaryServerInterceptor(base *zap.SugaredLogger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		logger := base.With(zap.String("api", info.FullMethod))
		logger.Info("[gRPC]")

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

		requestIds := md.Get("x-app-request-id")
		if len(requestIds) > 0 {
			logger = logger.With(zap.String("request_id", requestIds[0]))
		}

		// embed uuid into context
		_, tokenExempt := noNeedTokenSet[info.FullMethod]
		var suid *uuid.UUID
		if !tokenExempt {
			suid, err = getSuid(ctx)
			if err != nil {
				logger.Error("failed to get suid", zap.Error(err))
				return nil, err
			}
			logger = logger.With(zap.String("suid", suid.String()))
			ctx = context.WithValue(ctx, AuthTokenKey{}, suid)

			reloginRequired, err := requiresDailyRelogin(ctx, suid.String(), time.Now())
			if err != nil {
				logger.Error("failed checking daily login", zap.Error(err))
				if status.Code(err) != codes.Unknown {
					return nil, err
				}
				return nil, errInternal
			}
			if reloginRequired {
				header := responseHeader()
				if err := grpc.SetHeader(ctx, header); err != nil {
					logger.Error("failed setting response header", zap.Error(err))
					return nil, err
				}
				header = titleActionHeader("日付が変わりました")
				if err := grpc.SetHeader(ctx, header); err != nil {
					logger.Error("failed setting daily relogin header", zap.Error(err))
					return nil, err
				}
				return &papi.Empty{}, nil
			}
		}
		// thread logger to context
		ctx = mcontext.WithLogger(ctx, logger)

		// set metadata (header)
		header := responseHeader()
		if err := grpc.SetHeader(ctx, header); err != nil {
			logger.Error("failed setting response header", zap.Error(err))
			return nil, err
		}

		var requestId string
		if len(requestIds) > 0 {
			if tokenExempt {
				return nil, status.Error(codes.InvalidArgument, "x-app-request-id requires authentication")
			}
			requestId, err = parseAppRequestId(requestIds)
			if err != nil {
				return nil, err
			}
		}

		// invoke handler
		var m any
		if requestId == "" {
			m, err = handler(ctx, req)
		} else {
			err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
				if err := opuser.ReserveAppRequest(ctx, suid.String(), requestId, info.FullMethod); err != nil {
					return nil, err
				}
				m, err = handler(ctx, req)
				return nil, err
			})
			if errors.Is(err, opuser.ErrDuplicateAppRequest) {
				logger.Warn("rejected duplicate app request")
				return nil, status.Error(codes.AlreadyExists, "duplicate x-app-request-id")
			}
		}
		if err != nil {
			logger.Error("RPC failed with error", zap.Error(err))
		}
		// check if invoked API is not implemented
		code := status.Code(err)
		if code == codes.Unimplemented {
			header = titleActionHeader("未実装の機能です")
			if err := grpc.SetHeader(ctx, header); err != nil {
				logger.Error("failed setting unimplemented header", zap.Error(err))
				return nil, err
			}
			return &papi.Empty{}, nil
		}

		return m, abstractErrorMsg(err)
	}
}

func UnknownServiceHandler(base *zap.SugaredLogger) grpc.StreamHandler {
	return func(_ any, stream grpc.ServerStream) error {
		fullMethod, _ := grpc.MethodFromServerStream(stream)
		logger := base.With(zap.String("api", fullMethod))
		logger.Infof("[gRPC] %s", fullMethod)

		if err := stream.RecvMsg(&papi.Empty{}); err != nil {
			logger.Error("failed receiving unimplemented request", zap.Error(err))
			return err
		}
		if err := stream.SetHeader(responseHeader()); err != nil {
			logger.Error("failed setting response header", zap.Error(err))
			return err
		}
		if err := stream.SetHeader(titleActionHeader("未実装の機能です")); err != nil {
			logger.Error("failed setting unimplemented header", zap.Error(err))
			return err
		}
		return stream.SendMsg(&papi.Empty{})
	}
}

func responseHeader() metadata.MD {
	return metadata.New(map[string]string{
		"x-server-time": fmt.Sprintf("%d", utils.GetTimestamp()),
		"cache-control": "no-store",
	})
}

func titleActionHeader(message string) metadata.MD {
	// grpc-go base64-encodes raw values whose key ends in -bin. The
	// resulting HTTP/2 value is the value specified by the client.
	return metadata.New(map[string]string{
		"x-error-action":      "title",
		"x-error-message-bin": message,
		"x-error-code":        "1003",
	})
}

func requiresDailyRelogin(ctx context.Context, suid string, now time.Time) (bool, error) {
	userAuth, err := opuser.GetUserAuthByServerId(ctx, suid)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return false, errInvalidToken
		}
		return false, err
	}
	lastLogin := time.UnixMilli(userAuth.LastLoginAt).In(jstLocation)
	current := now.In(jstLocation)
	return lastLogin.Year() != current.Year() || lastLogin.YearDay() != current.YearDay(), nil
}

func parseAppRequestId(values []string) (string, error) {
	if len(values) != 1 || values[0] == "" {
		return "", status.Error(codes.InvalidArgument, "invalid x-app-request-id")
	}
	requestId, err := strconv.ParseUint(values[0], 10, 64)
	if err != nil {
		return "", status.Error(codes.InvalidArgument, "invalid x-app-request-id")
	}
	return strconv.FormatUint(requestId, 10), nil
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
