package test

import (
	"github.com/vertesan/solis-server/internal/db"
	"github.com/vertesan/solis-server/internal/server/rpc/codec"
	"github.com/vertesan/solis-server/internal/utils/config"
	"log"
	"os"
	"testing"

	"github.com/gofiber/fiber/v3/client"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func init() {
	var err error
	err = godotenv.Load("../.env")
	if err != nil {
		log.Fatalf("error loading .env file: %v", err)
	}

	// check test environment
	if testConfig.Env == Prd {
		testConfig.GrpcFqdn = testConfig.GrpcFqdnPrd
		testConfig.HttpUrl = testConfig.HttpUrlPrd
	} else {
		testConfig.GrpcFqdn = testConfig.GrpcFqdnDev
		testConfig.HttpUrl = testConfig.HttpUrlDev
	}

	var opts []grpc.DialOption

	// use codec (or not)
	if !config.IsNoCodec() {
		opts = append(opts, grpc.WithDefaultCallOptions(grpc.ForceCodec(&codec.QuaClientCodec{})))
	}

	// TLS credentials
	creds, err := credentials.NewClientTLSFromFile(config.GetGrpcCertPath(), "")
	if err != nil {
		log.Fatalf("cannot load certificate: %v", err)
	}

	// compression
	compression := grpc.WithDefaultCallOptions(grpc.UseCompressor("identity"))

	// append options
	opts = append(
		opts,
		grpc.WithTransportCredentials(creds),
		compression,
	)

	// create channel
	testConfig.GrpcConn, err = grpc.NewClient(testConfig.GrpcFqdn, opts...)
	if err != nil {
		log.Fatalf("did not connect to %q: %v", testConfig.GrpcFqdn, err)
	}

	// create http client
	testConfig.HttpClient = client.New()

	// initialize db connection
	if err := db.Connect(); err != nil {
		log.Fatalf("failed to initialize test: %v", err)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	if testConfig.GrpcConn != nil {
		testConfig.GrpcConn.Close()
	}
	os.Exit(code)
}
