package test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/codec"
	"github.com/vertesan/solis-server/internal/utils/config"
	"golang.org/x/net/http2"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

type grpcWireResponse struct {
	Headers metadata.MD
	Status  string
	Body    []byte
}

// Read the actual HTTP/2 body instead of using a generated unary client. A
// successful title/home action intentionally contains no response message,
// which grpc-go's unary client rejects as a cardinality violation.
func callGRPCWire(t *testing.T, ctx context.Context, method string, message proto.Message) grpcWireResponse {
	t.Helper()
	payload, err := (&codec.QuaClientCodec{}).Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)

	certificatePath, err := config.GetGrpcCertPath()
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := os.ReadFile(certificatePath)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificate) {
		t.Fatal("could not load gRPC test certificate")
	}
	transport := &http2.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+testConfig.GrpcFqdn+method, bytes.NewReader(frame))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/grpc")
	request.Header.Set("TE", "trailers")
	request.Header.Set("Grpc-Encoding", "identity")
	md, _ := metadata.FromOutgoingContext(ctx)
	for key, values := range md {
		for _, value := range values {
			if strings.HasSuffix(key, "-bin") {
				value = base64.RawStdEncoding.EncodeToString([]byte(value))
			}
			request.Header.Add(key, value)
		}
	}
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.ProtoMajor != 2 || response.StatusCode != http.StatusOK {
		t.Fatalf("%s returned HTTP/%d status %d", method, response.ProtoMajor, response.StatusCode)
	}
	const maximumBodySize = 4<<20 + 5
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumBodySize+1))
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > maximumBodySize {
		t.Fatalf("%s exceeded the wire test response-size limit", method)
	}
	headers := metadata.MD{}
	for key, values := range response.Header {
		key = strings.ToLower(key)
		for _, value := range values {
			if strings.HasSuffix(key, "-bin") {
				decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(value, "="))
				if err != nil {
					t.Fatalf("invalid binary metadata %s: %v", key, err)
				}
				value = string(decoded)
			}
			headers.Append(key, value)
		}
	}
	grpcStatus := response.Trailer.Get("Grpc-Status")
	if grpcStatus == "" {
		grpcStatus = response.Header.Get("Grpc-Status")
	}
	if grpcStatus == "" {
		t.Fatalf("%s omitted grpc-status", method)
	}
	return grpcWireResponse{Headers: headers, Status: grpcStatus, Body: body}
}

func assertGRPCWireAction(t *testing.T, response grpcWireResponse, action, code, message string) {
	t.Helper()
	if response.Status != "0" || len(response.Body) != 0 {
		t.Fatalf("action returned grpc-status=%q and %d body bytes; want status 0 and no message frame", response.Status, len(response.Body))
	}
	assertSingleMetadataValue(t, response.Headers, "x-error-action", action)
	assertSingleMetadataValue(t, response.Headers, "x-error-code", code)
	assertSingleMetadataValue(t, response.Headers, "x-error-message-bin", message)
}

func TestGRPCActionWireScenario(t *testing.T) {
	setupTestUser(t)
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	// Empty application messages are still real unary messages, unlike a
	// header-only action. Do not accidentally remove their five-byte frame.
	health := callGRPCWire(t, ctx, papi.Health_Check_FullMethodName, &papi.HealthCheckRequest{})
	if health.Status != "0" || !bytes.Equal(health.Body, []byte{0, 0, 0, 0, 0}) {
		t.Fatalf("normal empty response lost its frame: status=%s body length=%d", health.Status, len(health.Body))
	}

	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set("x-master-version", "outdated-master-wire-test")
	const requestID = "639252874932389999"
	md.Set("x-app-request-id", requestID)
	stale := callGRPCWire(t, metadata.NewOutgoingContext(ctx, md), papi.User_Get_FullMethodName, &papi.Empty{})
	assertGRPCWireAction(t, stale, "title", "1002", "マスターデータが更新されました")

	// The rejected request must not reserve its idempotency key. Reuse it with
	// the current master version and verify a normal framed response follows.
	current := callGRPCWire(t, metadata.AppendToOutgoingContext(ctx, "x-app-request-id", requestID), papi.User_Get_FullMethodName, &papi.Empty{})
	if current.Status != "0" || len(current.Body) <= 5 || current.Body[0] != 0 || int(binary.BigEndian.Uint32(current.Body[1:5])) != len(current.Body)-5 {
		t.Fatalf("normal response was not one complete message: status=%s body length=%d", current.Status, len(current.Body))
	}
	user := &papi.UserGetResponse{}
	err = (&codec.QuaClientCodec{}).Unmarshal(current.Body[5:], user)
	if err != nil || user.GetUser() == nil {
		t.Fatalf("normal response did not decode to the configured user: %v", err)
	}
	if len(current.Headers.Get("x-error-action")) != 0 {
		t.Fatal("normal response included action headers")
	}

	// Ordinary gRPC failures retain their non-OK status and no action headers.
	md, _ = metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Delete("x-auth-token")
	unauthenticated := callGRPCWire(t, metadata.NewOutgoingContext(ctx, md), papi.User_Get_FullMethodName, &papi.Empty{})
	if unauthenticated.Status != "16" || len(unauthenticated.Body) != 0 || len(unauthenticated.Headers.Get("x-error-action")) != 0 {
		t.Fatalf("ordinary error response changed: status=%s body length=%d", unauthenticated.Status, len(unauthenticated.Body))
	}
}
