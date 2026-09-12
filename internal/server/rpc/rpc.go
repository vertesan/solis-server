package rpc

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/codec"
	"github.com/vertesan/solis-server/internal/server/rpc/internal/interceptor"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/accessory"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/activity"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/auth"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/birthday"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/card"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/company"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/costume"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/diary"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/division"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/dokan"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/exchange"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/excursion"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/friend"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/gacha"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/gift"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/guild"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/hair"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/health"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/hierarchy"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/home"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/item"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/login_bonus"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/love"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/market"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/master"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/message"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/migration"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/mission"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/notice"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/photo"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/photo_panorama"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/profile"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/salary"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/shop"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/showcase"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/staff"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/story"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/system"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/telephone"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/user"
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
	unaryInterceptor := grpc.UnaryInterceptor(interceptor.UnaryServerInterceptor(base))
	unknownServiceHandler := grpc.UnknownServiceHandler(interceptor.UnknownServiceHandler(base))

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
		unaryInterceptor,
		unknownServiceHandler,
	)
	server := grpc.NewServer(opts...)
	papi.RegisterSystemServer(server, &system.SystemServer{})
	papi.RegisterMigrationServer(server, &migration.MigrationServer{})
	papi.RegisterAuthServer(server, &auth.AuthServer{})
	papi.RegisterMasterServer(server, &master.MasterServer{})
	papi.RegisterUserServer(server, &user.UserServer{})
	papi.RegisterHomeServer(server, &home.HomeServer{})
	papi.RegisterDokanServer(server, &dokan.DokanServer{})
	papi.RegisterNoticeServer(server, &notice.NoticeServer{})
	papi.RegisterStoryServer(server, &story.StoryServer{})
	papi.RegisterAccessoryServer(server, &accessory.AccessoryServer{})
	papi.RegisterItemServer(server, &item.ItemServer{})
	papi.RegisterCardServer(server, &card.CardServer{})
	papi.RegisterDivisionServer(server, &division.DivisionServer{})
	papi.RegisterExchangeServer(server, &exchange.ExchangeServer{})
	papi.RegisterShopServer(server, &shop.ShopServer{})
	papi.RegisterGiftServer(server, &gift.GiftServer{})
	papi.RegisterLoginBonusServer(server, &loginbonus.LoginBonusServer{})
	papi.RegisterCostumeServer(server, &costume.CostumeServer{})
	papi.RegisterHairServer(server, &hair.HairServer{})
	papi.RegisterDiaryServer(server, &diary.DiaryServer{})
	papi.RegisterMessageServer(server, &message.MessageServer{})
	papi.RegisterTelephoneServer(server, &telephone.TelephoneServer{})
	papi.RegisterMarketServer(server, &market.MarketServer{})
	papi.RegisterExcursionServer(server, &excursion.ExcursionServer{})
	papi.RegisterGachaServer(server, &gacha.GachaServer{})
	papi.RegisterActivityServer(server, &activity.ActivityServer{})
	papi.RegisterCompanyServer(server, &company.CompanyServer{})
	papi.RegisterHierarchyServer(server, &hierarchy.HierarchyServer{})
	papi.RegisterGuildServer(server, &guild.GuildServer{})
	papi.RegisterStaffServer(server, &staff.StaffServer{})
	papi.RegisterPhotoServer(server, &photo.PhotoServer{})
	papi.RegisterPhotoPanoramaServer(server, &photopanorama.PhotoPanoramaServer{})
	papi.RegisterShowcaseServer(server, &showcase.ShowcaseServer{})
	papi.RegisterProfileServer(server, &profile.ProfileServer{})
	papi.RegisterFriendServer(server, &friend.FriendServer{})
	papi.RegisterLoveServer(server, &love.LoveServer{})
	papi.RegisterHealthServer(server, &health.HealthServer{})
	papi.RegisterSalaryServer(server, &salary.SalaryServer{})
	papi.RegisterMissionServer(server, &mission.MissionServer{})
	papi.RegisterBirthdayServer(server, &birthday.BirthdayServer{})
	return server, nil
}
