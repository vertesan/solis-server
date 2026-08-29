package rpc

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/vertesan/solis-server/internal/proto/papi"
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

func StartGRPC(ctx context.Context, logger *zap.SugaredLogger) error {
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

	logger.Infof("grpc server listening at port: %v", grpcPort)
	if err := s.Serve(lis); err != nil && ctx.Err() == nil {
		return fmt.Errorf("failed to serve: %w", err)
	}
	return nil
}

func initServer(base *zap.SugaredLogger) (*grpc.Server, error) {
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
	papi.RegisterSystemServer(server, &ss.SystemServer{})
	papi.RegisterMigrationServer(server, &ss.MigrationServer{})
	papi.RegisterAuthServer(server, &ss.AuthServer{})
	papi.RegisterMasterServer(server, &ss.MasterServer{})
	papi.RegisterUserServer(server, &ss.UserServer{})
	papi.RegisterHomeServer(server, &ss.HomeServer{})
	papi.RegisterDokanServer(server, &ss.DokanServer{})
	papi.RegisterNoticeServer(server, &ss.NoticeServer{})
	papi.RegisterStoryServer(server, &ss.StoryServer{})
	papi.RegisterAccessoryServer(server, &ss.AccessoryServer{})
	papi.RegisterCardServer(server, &ss.CardServer{})
	papi.RegisterDivisionServer(server, &ss.DivisionServer{})
	papi.RegisterExchangeServer(server, &ss.ExchangeServer{})
	papi.RegisterShopServer(server, &ss.ShopServer{})
	papi.RegisterGiftServer(server, &ss.GiftServer{})
	papi.RegisterLoginBonusServer(server, &ss.LoginBonusServer{})
	papi.RegisterCostumeServer(server, &ss.CostumeServer{})
	papi.RegisterHairServer(server, &ss.HairServer{})
	papi.RegisterDiaryServer(server, &ss.DiaryServer{})
	papi.RegisterMessageServer(server, &ss.MessageServer{})
	papi.RegisterTelephoneServer(server, &ss.TelephoneServer{})
	papi.RegisterMarketServer(server, &ss.MarketServer{})
	papi.RegisterExcursionServer(server, &ss.ExcursionServer{})
	papi.RegisterGachaServer(server, &ss.GachaServer{})
	papi.RegisterActivityServer(server, &ss.ActivityServer{})
	papi.RegisterCompanyServer(server, &ss.CompanyServer{})
	papi.RegisterPhotoServer(server, &ss.PhotoServer{})
	return server, nil
}
