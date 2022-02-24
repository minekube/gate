package connect

import (
	"context"

	"go.minekube.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrEndpointNotFound indicates that an endpoint could not be found by EndpointRegistry.
var ErrEndpointNotFound = status.Error(codes.NotFound, "could not found endpoint")

// EndpointRegistry retrieves registered endpoints.
type EndpointRegistry interface {
	// Endpoint gets a registered endpoint by name.
	// If not found ErrEndpointNotFound is returned.
	Endpoint(ctx context.Context, name string) (Endpoint, error)
}

// EndpointDialer establishes a TunnelConn with an Endpoint.
//
// It proposes a session to the endpoint, waits for the endpoint to create a
// TunnelConn with the TunnelService listening at ServeOptions.PublicTunnelServiceAddr
// and returns the TunnelConn.
//
// It is recommended to always pass a timeout context because the endpoint might never
// create the TunnelConn with the TunnelService.
//
// Dial unblocks on the following events:
//  - If the endpoint has established a TunnelConn successfully.
//  - If the passed context is canceled, cleans up and cancels the session proposal.
//  - If the endpoint rejected the session proposal wrapping the given status reason in the returned error if present.
//  - If the endpoint's watcher has disconnected / was unregistered.
//
type EndpointDialer interface {
	DialEndpoint(ctx context.Context, endpoint string, proposal *connect.Session) (connect.TunnelConn, error)
}

// DialEndpoint establishes a TunnelConn with an Endpoint.
//
// It proposes a session to the endpoint, waits for the endpoint to create a
// TunnelConn with the TunnelService listening at ServeOptions.PublicTunnelServiceAddr
// and returns the TunnelConn.
//
// It is recommended to always pass a timeout context because the endpoint might never
// create the TunnelConn with the TunnelService.
//
// Dial unblocks on the following events:
//  - If the endpoint has established a TunnelConn successfully.
//  - If the passed context is canceled, cleans up and cancels the session proposal.
//  - If the endpoint rejected the session proposal wrapping the given status reason in the returned error if present.
//  - If the endpoint's watcher has disconnected / was unregistered.
//
func DialEndpoint(ctx context.Context, endpoint connect.Watcher, proposal *connect.Session) (TunnelConn, error) {

}
