package config

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-logr/logr"
	"go.minekube.com/connect"
	"go.minekube.com/connect/bedrockprincipal"
	"go.minekube.com/connect/ws"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/process"
	"go.minekube.com/gate/pkg/util/connectutil"
	"go.minekube.com/gate/pkg/util/uuid"
)

const mdOfflineMode = connect.MDEndpoint + "-offline-mode"

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

	principal, err := newPrincipalVerifier(c.BedrockPrincipal)
	if err != nil {
		return nil, err
	}

	return process.RunnableFunc(func(ctx context.Context) error {
		if c.Name == "" {
			c.Name = randomEndpointName(ctx)
		}

		ph := proposalHandler{
			localAddr:          nil,
			connHandler:        connHandler.HandleConn,
			enforcePassthrough: c.EnforcePassthrough,
			principal:          principal,
		}
		ctx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithName("proposal"))

		return retryingRunnable(process.RunnableFunc(func(ctx context.Context) error {
			// Load auth token
			token, err := loadToken(c.TokenFilePath)
			if err != nil {
				return err
			}
			dialCtx := metadata.AppendToOutgoingContext(ctx,
				"Authorization", "Bearer "+token,
				connect.MDEndpoint, c.Name,
				connect.MDPrefix+"connector", "gate",
			)
			if c.AllowOfflineModePlayers {
				dialCtx = metadata.AppendToOutgoingContext(dialCtx, mdOfflineMode, "true")
			}
			// Re-evaluated on every (re)connect: a not-ready verifier
			// downgrades to no capability advertisement.
			if caps := principal.capabilities(); len(caps) != 0 {
				dialCtx = metadata.AppendToOutgoingContext(dialCtx,
					connect.MDPrefix+"capabilities", strings.Join(caps, ","))
			}

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
			// A 401 from the watch service means the server currently rejects
			// this (endpoint, token) — e.g. a displaced connector. Mark it so
			// retryingRunnable switches to a cold recovery probe instead of
			// hammering the server.
			if res, ok := ws.DialErrorResponse(err); ok && res.StatusCode == http.StatusUnauthorized {
				err = &authRejectedError{endpoint: c.Name, err: err}
			}
			if ctx.Err() == nil {
				// Reconnect to WatchService. retryingRunnable backs off
				// exponentially and stops logging after 5 consecutive
				// failures; repeated 401s use a much slower recovery cadence.
				if err == nil {
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
	principal          *principalVerifier
}

func (h *proposalHandler) handle(ctx context.Context, proposal connect.SessionProposal) {
	// Log only non-identity values: no player name, address, profile or
	// principal envelope material may reach logs at any verbosity.
	log := logr.FromContextOrDiscard(ctx).
		WithName("session").
		WithValues("session", proposal.Session().GetId()).
		WithValues("passthrough", proposal.Session().GetAuth().GetPassthrough())
	ctx = logr.NewContext(ctx, log)

	log.Info("received session proposal")
	tc := &tunnelCreator{proposalHandler: h}
	if err := tc.handle(ctx, proposal); err != nil {
		log.Info("rejecting session proposal", "reason", err)
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

	if t.enforcePassthrough && !proposal.Session().GetAuth().GetPassthrough() {
		return status.Error(codes.Unauthenticated, "only allowing pass-through connections")
	}

	wire, err := connectutil.ExtractSessionPrincipalWire(proposal.Session())
	if err != nil {
		return status.Error(codes.InvalidArgument, "session proposal carries invalid bedrock principal fields")
	}

	log := logr.FromContextOrDiscard(ctx)

	var principal bedrockprincipal.VerifiedBedrockPrincipal
	if wire.HasEnvelope() {
		// Verify exactly once; a failure rejects the proposal with only the
		// bounded category and never falls back to the proposed profile.
		principal, err = t.principal.verify(ctx, proposal.Session().GetId(), wire)
		if err != nil {
			return status.Error(codes.Unauthenticated, principalErrorCategory(err))
		}
		log.Info("verified bedrock principal",
			"kid", principal.Verification().KID,
			"subjectKind", string(principal.SubjectKind()))
	} else if t.principal != nil && wire.IsBedrock() {
		// This endpoint requires verified Bedrock principals; a Bedrock
		// session without an envelope must not be admitted on proposed
		// profile data alone.
		return status.Error(codes.Unauthenticated, "bedrock session proposal is missing a signed principal envelope")
	}

	var gp *profile.GameProfile
	switch {
	case principal != nil:
		// Exactly one verified profile is applied: the one produced by the
		// Connect SDK verifier. The proposal's own profile is never consulted
		// for a session that carries a principal envelope.
		verified := principal.EffectiveGameProfile()
		gp = &profile.GameProfile{ID: uuid.UUID(verified.UUID), Name: verified.Name}
	case !proposal.Session().GetAuth().GetPassthrough():
		gp, err = convertProposedGameProfile(proposal.Session().GetPlayer().GetProfile())
		if err != nil {
			return status.Errorf(codes.InvalidArgument,
				"session proposal provided an invalid player game profile: %v", err)
		}
	}
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

	conn := wrapTunnelSession(tunnel, proposal.Session(), gp, principal)

	log.Info("established tunnel for session")
	t.connHandler(conn)
	return nil
}

// wrapTunnelSession layers the session, game profile and verified principal
// onto the tunnel connection. The outermost wrapper must keep the game profile
// visible to netmc.Assert, which only unwraps via a Conn() net.Conn method.
func wrapTunnelSession(
	tunnel connect.Tunnel,
	s *connect.Session,
	gp *profile.GameProfile,
	principal bedrockprincipal.VerifiedBedrockPrincipal,
) connectutil.TunnelSession {
	var conn connectutil.TunnelSession = &tunnelConnWithSession{Tunnel: tunnel, s: s}
	switch {
	case principal != nil:
		conn = &tunnelConnWithPrincipal{TunnelSession: conn, gp: gp, principal: principal}
	case gp != nil:
		conn = &tunnelConnWithGameProfile{TunnelSession: conn, gp: gp}
	}
	return conn
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
	tunnelConnWithPrincipal struct {
		connectutil.TunnelSession
		gp        *profile.GameProfile
		principal bedrockprincipal.VerifiedBedrockPrincipal
	}
)

var (
	_ proxy.GameProfileProvider             = (*tunnelConnWithGameProfile)(nil)
	_ proxy.GameProfileProvider             = (*tunnelConnWithPrincipal)(nil)
	_ proxy.ConnectTunnelIngress            = (*tunnelConnWithSession)(nil)
	_ proxy.ConnectTunnelIngress            = (*tunnelConnWithGameProfile)(nil)
	_ proxy.ConnectTunnelIngress            = (*tunnelConnWithPrincipal)(nil)
	_ connectutil.VerifiedPrincipalProvider = (*tunnelConnWithPrincipal)(nil)
)

func (t *tunnelConnWithGameProfile) GameProfile() *profile.GameProfile { return t.gp }
func (t *tunnelConnWithPrincipal) GameProfile() *profile.GameProfile   { return t.gp }
func (t *tunnelConnWithSession) Session() *connect.Session             { return t.s }
func (*tunnelConnWithSession) IsConnectTunnelIngress() bool            { return true }
func (*tunnelConnWithGameProfile) IsConnectTunnelIngress() bool        { return true }
func (*tunnelConnWithPrincipal) IsConnectTunnelIngress() bool          { return true }
func (t *tunnelConnWithPrincipal) VerifiedPrincipal() bedrockprincipal.VerifiedBedrockPrincipal {
	return t.principal
}

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
