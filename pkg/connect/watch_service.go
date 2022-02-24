package connect

import (
	"context"
	"errors"

	"go.minekube.com/connect"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.minekube.com/gate/pkg/runtime/logr"
)

// Endpoint is a server endpoint used to propose sessions or receive rejections.
type Endpoint interface {
	Name() string   // The endpoint name of this Endpoint.
	connect.Watcher // The watcher representing this Endpoint.
}

// EndpointRegistrar registers or unregisters endpoints.
type EndpointRegistrar interface {
	// Register registers an Endpoint.
	Register(Endpoint) error
	// Unregister unregisters an Endpoint by its name and returns true if it was found.
	Unregister(name string) error
}

// WatchServiceOptions are options for NewWatchService.
type WatchServiceOptions struct {
	EndpointRegistrar EndpointRegistrar // Registry used to un-/register endpoints
	Log               logr.Logger       // Optional logger
}

// NewWatchService returns a new watch service.
func NewWatchService(opts WatchServiceOptions) (*connect.WatchService, error) {
	if opts.EndpointRegistrar == nil {
		return nil, errors.New("missing endpoint registrar")
	}
	if opts.Log == nil {
		opts.Log = logr.NopLog
	}
	s := &watchStarter{WatchServiceOptions: opts}
	ws := &connect.WatchService{StartWatch: s.start}
	return ws, ws.Valid()
}

type watchStarter struct {
	WatchServiceOptions
}

func (s *watchStarter) start(watcher connect.Watcher) error {
	name := valueFrom(watcher.Context(), connect.MDEndpoint, metadata.FromIncomingContext)
	if name == "" {
		return status.Errorf(codes.InvalidArgument, "missing request metadata %q", connect.MDEndpoint)
	}
	e := &endpoint{
		Watcher: watcher,
		name:    name,
	}
	if err := s.EndpointRegistrar.Register(e); err != nil {
		return err
	}
	defer func() {
		err := s.EndpointRegistrar.Unregister(name)
		if err != nil {
			s.Log.Error(err, "Could not unregister endpoint", "name", e.Name())
		}
	}()
	<-watcher.Context().Done()
	return nil
}

type endpoint struct {
	connect.Watcher
	name string
}

func (s *endpoint) Name() string { return s.name }

func valueFrom(ctx context.Context, key string, mdFn func(ctx context.Context) (metadata.MD, bool)) string {
	md, _ := mdFn(ctx)
	if s := md.Get(key); len(s) != 0 {
		return s[0]
	}
	return ""
}
