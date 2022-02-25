package connect

import (
	"go.minekube.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// InboundTunnel is a tunnel initiated by a TunnelService client (server).
type InboundTunnel interface {
	connect.InboundTunnel
	SessionID() string
}

// TunnelAcceptor accepts InboundTunnel.
type TunnelAcceptor interface {
	AcceptTunnel(InboundTunnel) error
}

// AcceptInboundTunnel requires that an inbound tunnel provides a session id metadata and calls the TunnelAcceptor.
func AcceptInboundTunnel(acceptor TunnelAcceptor) func(connect.InboundTunnel) error {
	return func(tunnel connect.InboundTunnel) error {
		sessionID := valueFrom(tunnel.Context(), connect.MDSession, metadata.FromIncomingContext)
		if sessionID == "" {
			return status.Errorf(codes.InvalidArgument, "missing request metadata %q", connect.MDSession)
		}
		return acceptor.AcceptTunnel(&inboundTunnel{InboundTunnel: tunnel, sessionID: sessionID})
	}
}

type inboundTunnel struct {
	connect.InboundTunnel
	sessionID string
}

func (i *inboundTunnel) SessionID() string { return i.sessionID }
