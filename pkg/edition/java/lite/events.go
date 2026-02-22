package lite

import (
	"net"
	"net/netip"
	"time"
)

// ForwardEndReason describes why a Lite forward terminated.
type ForwardEndReason string

const (
	ClientClosed         ForwardEndReason = "ClientClosed"
	BackendClosed        ForwardEndReason = "BackendClosed"
	BackendConnectFailed ForwardEndReason = "BackendConnectFailed"
	Timeout              ForwardEndReason = "Timeout"
	Shutdown             ForwardEndReason = "Shutdown"
	Error                ForwardEndReason = "Error"
)

// ForwardStartedEvent is emitted when a Lite TCP forward starts.
type ForwardStartedEvent struct {
	ConnectionID string
	ClientIP     netip.Addr
	ClientAddr   net.Addr
	BackendAddr  net.Addr
	Host         string
	RouteID      string
	StartedAt    time.Time
}

// ForwardEndedEvent is emitted when a Lite TCP forward ends.
type ForwardEndedEvent struct {
	ConnectionID string
	ClientIP     netip.Addr
	ClientAddr   net.Addr
	BackendAddr  net.Addr
	Host         string
	RouteID      string
	StartedAt    time.Time
	EndedAt      time.Time
	Reason       ForwardEndReason
}
