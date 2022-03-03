package connectutil

import (
	"context"

	"go.minekube.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Endpoint is an endpoint (server) used to propose sessions or receive rejections.
type Endpoint interface {
	Name() string         // The endpoint name.
	connect.EndpointWatch // The watching endpoint.
}

// EndpointListener accepts an Endpoint and should block until the
// endpoint's context is canceled or an error occurred while accepting.
type EndpointListener interface {
	AcceptEndpoint(context.Context, Endpoint) error
}

// RequireEndpointName requires that an EndpointWatch provides the name in metadata.
func RequireEndpointName(ln EndpointListener) connect.EndpointListener {
	return acceptEndpoint(func(ctx context.Context, watch connect.EndpointWatch) error {
		name := metaVal(ctx, connect.MDEndpoint, metadata.FromIncomingContext)
		if name == "" {
			return status.Errorf(codes.InvalidArgument, "missing request metadata %s", connect.MDEndpoint)
		}
		return ln.AcceptEndpoint(ctx, &endpoint{EndpointWatch: watch, name: name})
	})
}

type endpoint struct {
	connect.EndpointWatch
	name string
}

func (s *endpoint) Name() string   { return s.name }
func (s *endpoint) String() string { return s.name }

func metaVal(ctx context.Context, key string, mdFn func(ctx context.Context) (metadata.MD, bool)) string {
	md, _ := mdFn(ctx)
	if s := md.Get(key); len(s) != 0 {
		return s[0]
	}
	return ""
}

type acceptEndpoint func(ctx context.Context, watch connect.EndpointWatch) error

func (fn acceptEndpoint) AcceptEndpoint(ctx context.Context, watch connect.EndpointWatch) error {
	return fn(ctx, watch)
}
