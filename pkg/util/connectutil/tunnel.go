package connectutil

import (
	"context"

	"go.minekube.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TunnelSession is a tunnel with its session.
type TunnelSession interface {
	connect.Tunnel
	Session() *connect.Session
}

// InboundTunnel is a tunnel with its session id.
type InboundTunnel interface {
	connect.Tunnel
	SessionID() string
}

// TunnelListener accepts InboundTunnel.
type TunnelListener interface {
	AcceptTunnel(context.Context, InboundTunnel) error
}

// RequireTunnelSessionID requires that a tunnel provides session id metadata.
func RequireTunnelSessionID(ln TunnelListener) connect.TunnelListener {
	return acceptTunnel(func(ctx context.Context, tun connect.Tunnel) error {
		sessionID := metaVal(ctx, connect.MDSession, metadata.FromIncomingContext)
		if sessionID == "" {
			return status.Errorf(codes.InvalidArgument, "missing request metadata %s", connect.MDSession)
		}
		return ln.AcceptTunnel(ctx, &tunnel{Tunnel: tun, sessionID: sessionID})
	})
}

type tunnel struct {
	connect.Tunnel
	sessionID string
}

func (i *tunnel) SessionID() string { return i.sessionID }

type acceptTunnel func(ctx context.Context, tunnel connect.Tunnel) error

func (fn acceptTunnel) AcceptTunnel(ctx context.Context, tunnel connect.Tunnel) error {
	return fn(ctx, tunnel)
}
