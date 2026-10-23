// Package transport adapts the game's header-only action responses to gRPC.
package transport

import (
	"reflect"

	"google.golang.org/grpc"
)

// NewUnaryRegistrar keeps generated unary handlers and their interceptor, but
// gives them the ServerStream response lifecycle. grpc-go's unary dispatcher
// always sends one message (including a five-byte frame for a nil payload).
// The game instead requires zero messages for header-only title/home actions.
//
// A nil response with nil error means no response message. A non-nil empty
// protobuf is still a real response and must retain its normal gRPC frame.
// Clients remain unary; no protobufs or method names are changed. Deliberately
// bodyless successes are game-specific: strict Go unary clients reject them.
func NewUnaryRegistrar(server grpc.ServiceRegistrar, interceptor grpc.UnaryServerInterceptor) grpc.ServiceRegistrar {
	return &unaryRegistrar{server: server, interceptor: interceptor}
}

type unaryRegistrar struct {
	server      grpc.ServiceRegistrar
	interceptor grpc.UnaryServerInterceptor
}

func (r *unaryRegistrar) RegisterService(description *grpc.ServiceDesc, implementation any) {
	// Build an independent registration; never modify generated descriptors or
	// their backing slices. Existing genuine streaming methods are left intact.
	registration := grpc.ServiceDesc{
		ServiceName: description.ServiceName,
		HandlerType: description.HandlerType,
		Metadata:    description.Metadata,
		Streams:     append([]grpc.StreamDesc(nil), description.Streams...),
	}
	for _, method := range description.Methods {
		registration.Streams = append(registration.Streams, grpc.StreamDesc{
			StreamName: method.MethodName,
			// Both flags intentionally remain false: receive exactly one request
			// and do not advertise these methods as streaming in service info.
			Handler: func(server any, stream grpc.ServerStream) error {
				response, err := method.Handler(server, stream.Context(), stream.RecvMsg, r.interceptor)
				if err != nil {
					return err
				}
				if response == nil || (reflect.ValueOf(response).Kind() == reflect.Pointer && reflect.ValueOf(response).IsNil()) {
					// gRPC flushes pending headers and writes status OK on return.
					// Without SendMsg there is no DATA payload or message prefix.
					return nil
				}
				return stream.SendMsg(response)
			},
		})
	}
	r.server.RegisterService(&registration, implementation)
}
