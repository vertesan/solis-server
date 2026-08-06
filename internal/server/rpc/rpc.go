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

func StartGRPC(ctx context.Context, logger *zap.Logger) {
	grpcPort := config.GetGrpcPort()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		logger.Fatal("failed to start listening", zap.Error(err))
	}
	s := initServer(logger)

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
		logger.Fatal("failed to serve", zap.Error(err))
	}
}

func initServer(base *zap.Logger) *grpc.Server {
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
		creds, err := credentials.NewServerTLSFromFile(config.GetGrpcCertPath(), config.GetGrpcCertKeyPath())
		if err != nil {
			base.Fatal("failed to generate credentials", zap.Error(err))
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
	return server
}
