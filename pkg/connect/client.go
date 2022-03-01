package connect

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.minekube.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/util/uuid"
)

// TunnelConn is a tunnel connection session.
type TunnelConn interface {
	connect.TunnelConn
	Session() *connect.Session
}

// WatchOptions are Watch options.
type WatchOptions struct {
	Name              string                     // The name of this watching server endpoint.
	Client            connect.WatchServiceClient // The WatchService client to watch for session proposals.
	ConnHandler       func(conn TunnelConn)      // Called in parallel when a new tunnel connection is successfully established.
	TunnelDialOptions []grpc.DialOption          // Dial options passed when dialing TunnelService for a new tunnel session.
	Log               logr.Logger                // Optional logger
}

// DialWatchService dials the WatchService at the target address and
// returns the watch client and the grpc.ClientConn that can be used
// for closing the underlying client connection.
func DialWatchService(ctx context.Context, target string, opts ...grpc.DialOption) (connect.WatchServiceClient, *grpc.ClientConn, error) {
	opts = append(opts, grpc.WithBlock())
	cc, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("could not dial %q: %w", target, err)
	}
	return connect.NewWatchServiceClient(cc), cc, nil
}

// Watch registers an endpoint named WatchOptions.Name and starts watching
// for session proposals from the WatchService to create tunnel connections
// and passing them to WatchOptions.ConnHandler in parallel.
//
// Watch blocks until the provided context is canceled, if an option in
// WatchOptions is invalid or if it could not dial the WatchService using
// the provided client.
func Watch(ctx context.Context, opts WatchOptions) error {
	// if opts.Client == nil {
	// 	return errors.New("missing WatchServiceClient")
	// }
	if opts.Name == "" {
		return errors.New("missing name")
	}
	if opts.ConnHandler == nil {
		return errors.New("missing connection handler")
	}
	if opts.Log == nil {
		opts.Log = logr.NopLog
	}
	opts.TunnelDialOptions = append(opts.TunnelDialOptions, grpc.WithBlock())

	w := &watcher{WatchOptions: opts}

	opts.Log.Info("Watching for session proposals...")
	ctx = metadata.AppendToOutgoingContext(ctx, connect.MDEndpoint, opts.Name)
	return connect.WatchWebsocket(ctx, "wss://grpc.minekube.com/watchService", connect.WatchWebsocketOptions{
		HTTPHeader: http.Header{
			connect.MDEndpoint: []string{opts.Name},
		},
		HandshakeResult: func(res *http.Response) error {
			return nil
		},
		Callback: func(proposal connect.SessionProposal) error {
			go w.callback(proposal)
			return nil
		},
	})
	// return connect.Watch(ctx, connect.WatchOptions{
	// 	Client: opts.Client,
	// 	Callback: func(proposal connect.SessionProposal) error {
	// 		go w.callback(proposal)
	// 		return nil
	// 	},
	// })
}

type watcher struct {
	WatchOptions
}

func (w *watcher) callback(proposal connect.SessionProposal) {
	tc := &tunnelCreator{
		log: w.Log.
			WithName("session").
			WithName(proposal.Session().GetId()).
			WithValues("username", proposal.Session().GetPlayer().GetProfile().GetName()),
		dialOpts:    w.TunnelDialOptions,
		localAddr:   w.Name,
		connHandler: w.ConnHandler,
	}
	if err := tc.handle(context.Background(), proposal); err != nil {
		_ = proposal.Reject(status.FromContextError(err).Proto())
	}
}

type tunnelCreator struct {
	log         logr.Logger
	dialOpts    []grpc.DialOption
	localAddr   string
	connHandler func(conn TunnelConn)
}

func (t *tunnelCreator) handle(ctx context.Context, proposal connect.SessionProposal) (err error) {
	tunnelSvcAddr := proposal.Session().GetTunnelServiceAddr()
	if tunnelSvcAddr == "" {
		return status.Error(codes.InvalidArgument, "session proposal is missing tunnel service address")
	}
	var gp *profile.GameProfile
	if !proposal.Session().GetAuth().GetPassthrough() {
		gp, err = convertProposedGameProfile(proposal.Session().GetPlayer().GetProfile())
		if err != nil {
			return status.Errorf(codes.InvalidArgument,
				"session proposal provided an invalid player game profile: %v", err)
		}
	}

	// Dial TunnelService
	// dialCtx, cancel := context.WithTimeout(ctx, time.Minute)
	// defer cancel()
	// var tunnelCC *grpc.ClientConn
	// tunnelCC, err = grpc.DialContext(dialCtx, tunnelSvcAddr, t.dialOpts...)
	// if err != nil {
	// 	return fmt.Errorf("error dialing tunnel service at %q: %w", tunnelSvcAddr, err)
	// }

	t.log.Info("Establishing tunnel for proposed session", "tunnelServiceAddr", tunnelSvcAddr)

	// Create tunnel connection
	ctx = metadata.AppendToOutgoingContext(ctx, connect.MDSession, proposal.Session().GetId())
	tc, err := connect.TunnelWebsocket(ctx, tunnelSvcAddr, connect.TunnelWebsocketOptions{
		LocalAddr:  t.localAddr,
		RemoteAddr: proposal.Session().GetPlayer().GetAddr(),
		HTTPHeader: http.Header{
			connect.MDSession: []string{proposal.Session().GetId()},
		},
	})
	// tc, err := connect.Tunnel(ctx, connect.TunnelOptions{
	// 	TunnelClient:  connect.NewTunnelServiceClient(tunnelCC),
	// 	LocalAddr:  t.localAddr,
	// 	RemoteAddr: proposal.Session().GetPlayer().GetAddr(),
	// })
	if err != nil {
		return err
	}

	var conn TunnelConn = &tunnelConnWithSession{TunnelConn: tc, s: proposal.Session()}
	if gp != nil {
		conn = &tunnelConnWithGameProfile{TunnelConn: conn, gp: gp}
	}

	t.log.Info("Established tunnel for session")
	t.connHandler(conn)
	return nil
}

type (
	tunnelConnWithSession struct {
		connect.TunnelConn
		s *connect.Session
	}
	tunnelConnWithGameProfile struct {
		TunnelConn
		gp *profile.GameProfile
	}
)

var _ proxy.GameProfileProvider = (*tunnelConnWithGameProfile)(nil)

func (t *tunnelConnWithGameProfile) GameProfile() *profile.GameProfile { return t.gp }
func (t *tunnelConnWithSession) Session() *connect.Session             { return t.s }

// converts the proposed player game profile to the one understandable by Gate
func convertProposedGameProfile(p *connect.GameProfile) (*profile.GameProfile, error) {
	if p.GetName() == "" {
		return nil, errors.New("missing username")
	}
	id, err := uuid.Parse(p.GetId())
	if err != nil {
		return nil, fmt.Errorf("invalid player id: %w", err)
	}
	props := make([]profile.Property, len(p.Properties))
	for i, prop := range p.Properties {
		props[i] = profile.Property{
			Name:      prop.GetName(),
			Value:     prop.GetValue(),
			Signature: prop.GetSignature(),
		}
	}
	return &profile.GameProfile{
		ID:         id,
		Name:       p.GetName(),
		Properties: props,
	}, nil
}
