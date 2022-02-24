package connect

import (
	"go.minekube.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type InboundTunnel interface {
	connect.InboundTunnel
	SessionID() string
}

// TunnelAcceptor accepts InboundTunnel.
type TunnelAcceptor interface {
	AcceptTunnel(InboundTunnel) error // See TunnelService.AcceptTunnel
}

// NewTunnelService returns a new tunnel service.
func NewTunnelService(acceptor TunnelAcceptor) *connect.TunnelService {
	return &connect.TunnelService{AcceptTunnel: func(tunnel connect.InboundTunnel) error {
		sessionID := valueFrom(tunnel.Context(), connect.MDSession, metadata.FromIncomingContext)
		if sessionID == "" {
			return status.Errorf(codes.InvalidArgument, "missing request metadata %q", connect.MDSession)
		}
		return acceptor.AcceptTunnel(&inboundTunnel{InboundTunnel: tunnel, sessionID: sessionID})
	}}
}

type inboundTunnel struct {
	connect.InboundTunnel
	sessionID string
}

func (i *inboundTunnel) SessionID() string { return i.sessionID }
