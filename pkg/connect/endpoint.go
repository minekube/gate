package connect

import (
	"context"

	"go.minekube.com/connect"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Endpoint is an endpoint (server) used to propose sessions or receive rejections.
type Endpoint interface {
	Name() string   // The endpoint name of this Endpoint.
	connect.Watcher // The watcher representing this Endpoint.
}

// EndpointAcceptor accepts an Endpoint and should block until the
// endpoint's context is canceled or an error occurred while accepting.
type EndpointAcceptor interface {
	AcceptEndpoint(Endpoint) error
}

// AcceptEndpoint requires that a watcher provides endpoint name metadata and calls the EndpointAcceptor.
func AcceptEndpoint(acceptor EndpointAcceptor) func(connect.Watcher) error {
	return func(w connect.Watcher) error {
		name := valueFrom(w.Context(), connect.MDEndpoint, metadata.FromIncomingContext)
		if name == "" {
			return status.Errorf(codes.InvalidArgument, "missing request metadata %q", connect.MDEndpoint)
		}
		return acceptor.AcceptEndpoint(&endpoint{Watcher: w, name: name})
	}
}

type endpoint struct {
	connect.Watcher
	name string
}

func (s *endpoint) Name() string   { return s.name }
func (s *endpoint) String() string { return s.name }

func valueFrom(ctx context.Context, key string, mdFn func(ctx context.Context) (metadata.MD, bool)) string {
	md, _ := mdFn(ctx)
	if s := md.Get(key); len(s) != 0 {
		return s[0]
	}
	return ""
}
