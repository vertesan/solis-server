package test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/proto/papi"
	rpctransport "github.com/vertesan/solis-server/internal/server/rpc/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Exercise the actual HTTP/2 body, rather than a decoded protobuf response:
// an empty protobuf is a five-byte frame, not an absent response message.
func TestGRPCBodylessUnaryAdapter(t *testing.T) {
	var typedNil *papi.HealthCheckResponse
	cases := []struct {
		name         string
		action       string
		reply        any
		handlerError error
		malformed    string
	}{
		{name: "title", action: "title"},
		{name: "home", action: "home"},
		{name: "typed_nil", action: "title", reply: typedNil},
		{name: "normal_empty", reply: &papi.Empty{}},
		{name: "normal_payload", reply: &papi.HealthCheckRequest{Service: "payload remains intact"}},
		{name: "application_error", handlerError: status.Error(codes.FailedPrecondition, "test rejection")},
		{name: "missing_request", malformed: "missing"},
		{name: "multiple_requests", malformed: "multiple"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			service := &bodylessTestHealthServer{err: tc.handlerError}
			intercept := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
				calls.Add(1)
				if info.FullMethod != papi.Health_Check_FullMethodName || info.Server != service {
					return nil, status.Error(codes.Internal, "adapter changed unary method information")
				}
				request, ok := req.(*papi.HealthCheckRequest)
				if !ok || request.GetService() != tc.name {
					return nil, status.Error(codes.Internal, "adapter did not decode the request")
				}
				if tc.action != "" {
					if err := grpc.SetHeader(ctx, metadata.Pairs(
						"x-error-action", tc.action,
						"x-error-code", "1003",
						"x-error-message-bin", "日付が変わりました",
					)); err != nil {
						return nil, err
					}
					return tc.reply, nil
				}
				if _, err := handler(ctx, req); err != nil {
					return nil, err
				}
				return tc.reply, nil
			}
			server := grpc.NewServer()
			original := papi.Health_ServiceDesc
			originalMethods := append([]grpc.MethodDesc(nil), original.Methods...)
			papi.RegisterHealthServer(rpctransport.NewUnaryRegistrar(server, intercept), service)
			if len(papi.Health_ServiceDesc.Methods) != len(originalMethods) ||
				len(papi.Health_ServiceDesc.Streams) != len(original.Streams) ||
				papi.Health_ServiceDesc.ServiceName != original.ServiceName ||
				papi.Health_ServiceDesc.HandlerType != original.HandlerType ||
				papi.Health_ServiceDesc.Metadata != original.Metadata {
				t.Fatal("registration mutated the generated service descriptor")
			}
			for i, method := range originalMethods {
				registered := papi.Health_ServiceDesc.Methods[i]
				if method.MethodName != registered.MethodName || reflect.ValueOf(method.Handler).Pointer() != reflect.ValueOf(registered.Handler).Pointer() {
					t.Fatal("registration mutated a generated method descriptor")
				}
			}
			methods := server.GetServiceInfo()["api.Health"].Methods
			if len(methods) != 1 || methods[0].Name != "Check" || methods[0].IsClientStream || methods[0].IsServerStream {
				t.Fatalf("registration changed unary service information: %+v", methods)
			}
			httpServer := httptest.NewUnstartedServer(server)
			httpServer.EnableHTTP2 = true
			httpServer.StartTLS()
			t.Cleanup(httpServer.Close)
			t.Cleanup(server.Stop)
			client := httpServer.Client()
			client.Timeout = 5 * time.Second
			t.Cleanup(client.CloseIdleConnections)

			requestBody := bodylessTestFrame(t, &papi.HealthCheckRequest{Service: tc.name})
			if tc.malformed == "missing" {
				requestBody = nil
			} else if tc.malformed == "multiple" {
				requestBody = append(requestBody, requestBody...)
			}
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, httpServer.URL+papi.Health_Check_FullMethodName, bytes.NewReader(requestBody))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/grpc+proto")
			request.Header.Set("TE", "trailers")
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(io.LimitReader(response.Body, 1024))
			if err != nil {
				t.Fatal(err)
			}
			if response.ProtoMajor != 2 || response.StatusCode != http.StatusOK {
				t.Fatalf("unexpected transport response: %s %s", response.Proto, response.Status)
			}
			wantStatus := strconv.Itoa(int(status.Code(tc.handlerError)))
			if tc.malformed != "" {
				wantStatus = strconv.Itoa(int(codes.Internal))
			}
			grpcStatus := response.Trailer.Get("Grpc-Status")
			if grpcStatus == "" {
				grpcStatus = response.Header.Get("Grpc-Status")
			}
			if grpcStatus != wantStatus {
				t.Fatalf("gRPC status = %q, want %s; headers=%v trailers=%v", grpcStatus, wantStatus, response.Header, response.Trailer)
			}
			if tc.action != "" {
				if response.Header.Get("X-Error-Action") != tc.action || response.Header.Get("X-Error-Code") != "1003" {
					t.Fatalf("action metadata missing from initial headers: %v", response.Header)
				}
				message, err := base64.RawStdEncoding.DecodeString(response.Header.Get("X-Error-Message-Bin"))
				if err != nil || string(message) != "日付が変わりました" {
					t.Fatalf("incorrect action message encoding: decoded=%q error=%v", message, err)
				}
			}
			var wantBody []byte
			if tc.action == "" && tc.handlerError == nil && tc.malformed == "" {
				wantBody = bodylessTestFrame(t, tc.reply.(proto.Message))
			}
			if !bytes.Equal(body, wantBody) {
				t.Fatalf("HTTP response body = %x, want %x", body, wantBody)
			}
			wantInterceptorCalls := int32(1)
			if tc.malformed != "" {
				wantInterceptorCalls = 0
			}
			if calls.Load() != wantInterceptorCalls {
				t.Fatalf("unary interceptor called %d times, want %d", calls.Load(), wantInterceptorCalls)
			}
			wantHandlerCalls := int32(1)
			if tc.action != "" || tc.malformed != "" {
				wantHandlerCalls = 0
			}
			if service.calls.Load() != wantHandlerCalls {
				t.Fatalf("handler called %d times, want %d", service.calls.Load(), wantHandlerCalls)
			}
			if tc.name == "normal_empty" {
				// Ordinary generated unary clients must continue to receive exactly
				// one response, including legitimate empty protobuf successes.
				tlsConfig := client.Transport.(*http.Transport).TLSClientConfig.Clone()
				connection, err := grpc.NewClient(httpServer.Listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
				if err != nil {
					t.Fatal(err)
				}
				defer connection.Close()
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				message, err := papi.NewHealthClient(connection).Check(ctx, &papi.HealthCheckRequest{Service: tc.name})
				if err != nil || message == nil {
					t.Fatalf("ordinary unary client response = %v, error = %v", message, err)
				}
				if calls.Load() != 2 || service.calls.Load() != 2 {
					t.Fatalf("generated client call did not invoke interceptor and handler once: %d/%d", calls.Load(), service.calls.Load())
				}
			}
		})
	}
}

type bodylessTestHealthServer struct {
	papi.UnimplementedHealthServer
	calls atomic.Int32
	err   error
}

func (s *bodylessTestHealthServer) Check(context.Context, *papi.HealthCheckRequest) (*papi.HealthCheckResponse, error) {
	s.calls.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	return &papi.HealthCheckResponse{}, nil
}

func bodylessTestFrame(t *testing.T, message proto.Message) []byte {
	t.Helper()
	payload, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(fmt.Errorf("marshal test message: %w", err))
	}
	frame := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}
