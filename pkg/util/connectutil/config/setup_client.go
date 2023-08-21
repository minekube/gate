package config

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"go.minekube.com/connect"
	"go.minekube.com/connect/ws"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"nhooyr.io/websocket"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/process"
	"go.minekube.com/gate/pkg/util/connectutil"
	"go.minekube.com/gate/pkg/util/uuid"
)

// connectClient registers the endpoint and starts watching
// for session proposals from the WatchService to create tunnel connections
// and passing them to connHandler in parallel.
//
// Watch reconnects on disconnect.
func connectClient(c Config, connHandler ConnHandler) (process.Runnable, error) {
	if c.WatchServiceAddr == "" {
		return nil, errors.New("missing watch service address for listening to session proposals")
	}
	c.Name = strings.TrimSpace(c.Name)

	return process.RunnableFunc(func(ctx context.Context) error {
		if c.Name == "" {
			c.Name = randomEndpointName(ctx)
		}

		ph := proposalHandler{
			localAddr:          nil,
			connHandler:        connHandler.HandleConn,
			enforcePassthrough: c.EnforcePassthrough,
		}
		ctx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithName("proposal"))

		return retryingRunnable(process.RunnableFunc(func(ctx context.Context) error {
			// Load auth token
			token, err := loadToken(tokenFilename)
			if err != nil {
				return err
			}
			dialCtx := metadata.AppendToOutgoingContext(ctx,
				"Authorization", "Bearer "+token,
				connect.MDEndpoint, c.Name,
			)

			log := logr.FromContextOrDiscard(ctx)

			const timeout = time.Minute
			log.Info("connecting to watch service...",
				"endpoint", c.Name,
				"addr", c.WatchServiceAddr,
				"timeout", timeout.String())
			t := time.Now()

			dialCtx, cancel := context.WithTimeout(dialCtx, timeout)
			defer cancel()

			err = ws.ClientOptions{
				URL:         c.WatchServiceAddr,
				DialContext: dialCtx,
				Handshake: func(ctx context.Context, res *http.Response) (context.Context, error) {
					log.Info("connected", "took", time.Since(t).Round(time.Millisecond).String())
					return ctx, nil
				},
			}.Watch(ctx, func(proposal connect.SessionProposal) error {
				go ph.handle(ctx, proposal)
				return nil
			})
			if ctx.Err() == nil {
				// Reconnect to WatchService
				if err == nil {
					// TODO Backoff reconnect without logging an error after 5 times
					err = errors.New("disconnected by watch service")
					log.Info("session watcher disconnected by server, reconnecting", "after", time.Since(t))
				}
			} else if errors.Is(ctx.Err(), context.Canceled) {
				// Context canceled
				return nil
			}
			return err
		})).Start(ctx)

	}), nil
}

type proposalHandler struct {
	localAddr          net.Addr
	connHandler        func(net.Conn) // Called in parallel when a new tunnel connection is successfully established.
	enforcePassthrough bool
}

func (h *proposalHandler) handle(ctx context.Context, proposal connect.SessionProposal) {
	ctx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).
		WithName("session").
		WithValues("session", proposal.Session().GetId()).
		WithValues("username", proposal.Session().GetPlayer().GetProfile().GetName()),
	)
	tc := &tunnelCreator{proposalHandler: h}
	if err := tc.handle(ctx, proposal); err != nil {
		rejectCtx, cancel := context.WithTimeout(ctx, time.Second*20)
		defer cancel()
		_ = proposal.Reject(rejectCtx, status.FromContextError(err).Proto())
	}
}

type tunnelCreator struct {
	*proposalHandler
}

func (t *tunnelCreator) handle(ctx context.Context, proposal connect.SessionProposal) (err error) {
	// Validate proposal
	if proposal.Session().GetId() == "" {
		return status.Error(codes.InvalidArgument, "session proposal is missing id")
	}
	tunnelSvcAddr := proposal.Session().GetTunnelServiceAddr()
	if tunnelSvcAddr == "" {
		return status.Error(codes.InvalidArgument, "session proposal is missing tunnel service address")
	}
	if proposal.Session().GetPlayer().GetAddr() == "" {
		return status.Error(codes.InvalidArgument, "session proposal is missing player address")
	}
	var gp *profile.GameProfile
	if !proposal.Session().GetAuth().GetPassthrough() {
		if t.enforcePassthrough {
			return status.Error(codes.Unauthenticated, "only allowing pass-through connections")
		}
		gp, err = convertProposedGameProfile(proposal.Session().GetPlayer().GetProfile())
		if err != nil {
			return status.Errorf(codes.InvalidArgument,
				"session proposal provided an invalid player game profile: %v", err)
		}
	}

	log := logr.FromContextOrDiscard(ctx)
	log.Info("creating tunnel", "tunnelServiceAddr", tunnelSvcAddr)

	// Create tunnel connection
	ctx = metadata.AppendToOutgoingContext(ctx, connect.MDSession, proposal.Session().GetId())
	dialCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	ctx = connect.WithTunnelOptions(ctx, connect.TunnelOptions{
		LocalAddr:  t.localAddr,
		RemoteAddr: connect.Addr(proposal.Session().GetPlayer().GetAddr()),
	})

	tunnel, err := ws.ClientOptions{
		URL:         tunnelSvcAddr,
		DialContext: dialCtx,
		DialOptions: websocket.DialOptions{},
		Handshake: func(ctx context.Context, res *http.Response) (context.Context, error) {
			log.Info("tunnel connected")
			return ctx, nil
		},
	}.Tunnel(ctx)
	if err != nil {
		return status.Errorf(codes.Aborted, "could not connect to tunnel service: %v", err)
	}

	var conn connectutil.TunnelSession = &tunnelConnWithSession{Tunnel: tunnel, s: proposal.Session()}
	if gp != nil {
		conn = &tunnelConnWithGameProfile{TunnelSession: conn, gp: gp}
	}

	log.Info("established tunnel for session")
	t.connHandler(conn)
	return nil
}

type (
	tunnelConnWithSession struct {
		connect.Tunnel
		s *connect.Session
	}
	tunnelConnWithGameProfile struct {
		connectutil.TunnelSession
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
