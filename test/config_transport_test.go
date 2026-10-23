package test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc"
	"github.com/vertesan/solis-server/internal/server/rpc/codec"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/jwto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func TestJWTConfigurationScenario(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	private := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	publicBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	public := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicBytes})
	directory := t.TempDir()
	privatePath, publicPath := filepath.Join(directory, "private.pem"), filepath.Join(directory, "public.pem")
	for path, data := range map[string][]byte{privatePath: private, publicPath: public} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SOLIS_JWT_RSA_PRIV_KEY_PATH", privatePath)
	t.Setenv("SOLIS_JWT_RSA_PUB_KEY_PATH", publicPath)
	t.Setenv("SOLIS_JWT_HMAC_KEY", "test-only-jwt-hmac-key")
	if data, err := config.GetRSAPrivateKey(); err != nil || !bytes.Equal(data, private) {
		t.Fatal("renamed private-key path was not used")
	}
	if data, err := config.GetRSAPublicKey(); err != nil || !bytes.Equal(data, public) {
		t.Fatal("renamed public-key path was not used")
	}
	claims := jwt.MapClaims{"sub": testConfig.TestUserSuid, "exp": time.Now().Add(time.Minute).Unix()}
	jwtRSA, err := jwto.CreateToken(jwt.NewWithClaims(jwt.SigningMethodRS256, claims))
	if err != nil {
		t.Fatal(err)
	}
	if token, _, err := jwto.ParseRS256Token(jwtRSA); err != nil || !token.Valid {
		t.Fatalf("RSA signing/verification failed with renamed configuration: %v", err)
	}
	jwtHMAC, err := jwto.CreateGameAuthToken(jwt.NewWithClaims(jwt.SigningMethodHS256, claims))
	if err != nil {
		t.Fatal(err)
	}
	if token, _, err := jwto.ParseHS256Token(jwtHMAC); err != nil || !token.Valid {
		t.Fatalf("HMAC signing/verification failed with renamed configuration: %v", err)
	}
	t.Setenv("SOLIS_JWT_HMAC_KEY", "different-test-only-key")
	if _, _, err := jwto.ParseHS256Token(jwtHMAC); err == nil {
		t.Fatal("HMAC verification ignored the configured key")
	}
}

func TestGRPCRequiredConfigurationScenario(t *testing.T) {
	certificatePath, err := config.GetGrpcCertPath()
	if err != nil {
		t.Fatal(err)
	}
	for _, environment := range []string{"dev", "prd"} {
		t.Run(environment, func(t *testing.T) {
			t.Setenv("SOLIS_ENV", environment)
			t.Setenv("SOLIS_GRPC_PORT", "0")
			for _, missing := range []string{"SOLIS_GRPC_CERT_PUB_KEY_PATH", "SOLIS_GRPC_CERT_PRIV_KEY_PATH"} {
				t.Run(missing, func(t *testing.T) {
					t.Setenv(missing, "")
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					if err := rpc.StartGRPC(ctx, zap.NewNop().Sugar()); err == nil || !strings.Contains(err.Error(), missing) {
						t.Fatalf("missing TLS configuration did not reject startup: %v", err)
					}
				})
			}
			t.Run("invalid certificate", func(t *testing.T) {
				invalid := filepath.Join(t.TempDir(), "invalid.pem")
				if err := os.WriteFile(invalid, []byte("not a certificate"), 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("SOLIS_GRPC_CERT_PUB_KEY_PATH", invalid)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := rpc.StartGRPC(ctx, zap.NewNop().Sugar()); err == nil {
					t.Fatal("invalid certificate was accepted")
				}
			})
			t.Run("TLS and game codec required", func(t *testing.T) {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				_, port, err := net.SplitHostPort(listener.Addr().String())
				listener.Close()
				if err != nil {
					t.Fatal(err)
				}
				t.Setenv("SOLIS_GRPC_PORT", port)
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan error, 1)
				go func() { done <- rpc.StartGRPC(ctx, zap.NewNop().Sugar()) }()
				t.Cleanup(func() {
					cancel()
					select {
					case err := <-done:
						if err != nil {
							t.Errorf("test RPC server failed: %v", err)
						}
					case <-time.After(7 * time.Second):
						t.Error("test RPC server did not stop")
					}
				})
				creds, err := credentials.NewClientTLSFromFile(certificatePath, "")
				if err != nil {
					t.Fatal(err)
				}
				// localhost matches the development certificate used by the main
				// integration client; the selected port is private to this test.
				target := net.JoinHostPort("localhost", port)
				connection, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds), grpc.WithDefaultCallOptions(grpc.ForceCodec(&codec.QuaClientCodec{})))
				if err != nil {
					t.Fatal(err)
				}
				defer connection.Close()
				callCtx, callCancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, err = papi.NewHealthClient(connection).Check(callCtx, &papi.HealthCheckRequest{Service: "codec-and-TLS"}, grpc.WaitForReady(true))
				callCancel()
				if err != nil {
					t.Fatalf("TLS/game-codec request failed in %s: %v", environment, err)
				}
				plain, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.ForceCodec(&codec.QuaClientCodec{})))
				if err != nil {
					t.Fatal(err)
				}
				defer plain.Close()
				plainCtx, plainCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
				defer plainCancel()
				if _, err := papi.NewHealthClient(plain).Check(plainCtx, &papi.HealthCheckRequest{Service: "plaintext forbidden"}, grpc.WaitForReady(true)); err == nil {
					t.Fatal("plaintext gRPC was accepted")
				}
			})
		})
	}
}
