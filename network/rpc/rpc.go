package rpc

import (
	"fmt"
	"net"
	"time"
	"vertesan/solis-server/utils/config"
	"vertesan/solis-server/network/rpc/codec"
	ss "vertesan/solis-server/network/rpc/servers"
	pb "vertesan/solis-server/proto/papi"
	"vertesan/solis-server/utils/rich"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding"
	_ "google.golang.org/grpc/encoding/gzip"
)

func StartGRPC() {
	grpcPort := config.GetGrpcPort()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		rich.Fatal("failed to listen: %v", err)
	}
	s := InitServer()
	rich.Info("grpc server listening at port: %v", grpcPort)
	if err := s.Serve(lis); err != nil {
		rich.Fatal("failed to serve: %v", err)
	}
}

func InitServer() *grpc.Server {
	// register codec
	encoding.RegisterCodec(&codec.QuaServerCodec{})

	// declare server options
	var opts []grpc.ServerOption

	// production flavored operation (if any)
	env := config.GetEnv()
	if env == "prd" {
	}

	// unary interceptor
	interceptor := grpc.UnaryInterceptor(unaryServerInterceptor)

	// force (or not) customized codec
	if !config.IsNoCodec() {
		opts = append(opts, grpc.ForceServerCodec(&codec.QuaServerCodec{}))
	}

	// TLS credentials (only in dev environment)
	if !config.IsNoCert() {
		creds, err := credentials.NewServerTLSFromFile(config.GetGrpcCertPath(), config.GetGrpcCertKeyPath())
		if err != nil {
			rich.Fatal("failed to generate credentials: %v", err)
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
