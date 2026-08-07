package rpc

import (
	"context"
	"fmt"
	"net"
	"time"

	pb "github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/codec"
	"github.com/vertesan/solis-server/internal/server/rpc/internal/interceptor"
	ss "github.com/vertesan/solis-server/internal/server/rpc/servers"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.uber.org/zap"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding"
	_ "google.golang.org/grpc/encoding/gzip"
)

func StartGRPC(ctx context.Context, logger *zap.Logger) error {
	grpcPort := config.GetGrpcPort()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		return fmt.Errorf("failed to start listening: %w", err)
	}
	s, err := initServer(logger)
	if err != nil {
		return err
	}

	// spwan a thread listening to shutdown signal
	go func() {
		<-ctx.Done()
		stopped := make(chan struct{})
		go func() {
			s.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			s.Stop()
		}
	}()

	logger.Sugar().Infof("grpc server listening at port: %v", grpcPort)
	if err := s.Serve(lis); err != nil && ctx.Err() == nil {
		return fmt.Errorf("failed to serve: %w", err)
	}
	return nil
}

func initServer(base *zap.Logger) (*grpc.Server, error) {
	// register codec
	encoding.RegisterCodec(&codec.QuaServerCodec{})

	// declare server options
	var opts []grpc.ServerOption

	// production flavored operation (if any)
	env := config.GetEnv()
	if env == "prd" {
	}

	// unary interceptor
	interceptor := grpc.UnaryInterceptor(interceptor.UnaryServerInterceptor(base))

	// force (or not) customized codec
	if !config.IsNoCodec() {
		opts = append(opts, grpc.ForceServerCodec(&codec.QuaServerCodec{}))
	}

	// TLS credentials (only in dev environment)
	if !config.IsNoCert() {
		certPath, err := config.GetGrpcCertPath()
		if err != nil {
			return nil, err
		}
		keyPath, err := config.GetGrpcCertKeyPath()
		if err != nil {
			return nil, err
		}
		creds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to generate credentials: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	opts = append(
		opts,
		grpc.ConnectionTimeout(time.Second*10),
		interceptor,
	)
	server := grpc.NewServer(opts...)
	pb.RegisterSystemServer(server, &ss.SystemServer{})
	pb.RegisterMigrationServer(server, &ss.MigrationServer{})
	pb.RegisterAuthServer(server, &ss.AuthServer{})
	pb.RegisterMasterServer(server, &ss.MasterServer{})
	pb.RegisterUserServer(server, &ss.UserServer{})
	pb.RegisterHomeServer(server, &ss.HomeServer{})
	pb.RegisterDokanServer(server, &ss.DokanServer{})
	return server, nil
}
