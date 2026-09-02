package lite

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	connectiontelemetry "go.minekube.com/gate/pkg/telemetry/connection"
)

func TestPipeAccountsBothLiteIOCopyDirectionsAtClientBoundary(t *testing.T) {
	client, gateSide := net.Pipe()
	backend, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = gateSide.Close(); _ = backend.Close(); _ = server.Close() })

	collector := newLiteTelemetryCollector()
	ctx, session := connectiontelemetry.Start(context.Background(), collector)
	tracked := session.Attach(gateSide)
	done := make(chan struct{})
	go func() { pipe(logr.Discard(), tracked, backend); close(done) }()

	fromClient := []byte("client-to-backend")
	if _, err := client.Write(fromClient); err != nil {
		t.Fatal(err)
	}
	gotClient := make([]byte, len(fromClient))
	if _, err := io.ReadFull(server, gotClient); err != nil || string(gotClient) != string(fromClient) {
		t.Fatalf("backend received %q, %v", gotClient, err)
	}

	fromBackend := []byte("backend-to-client")
	if _, err := server.Write(fromBackend); err != nil {
		t.Fatal(err)
	}
	gotBackend := make([]byte, len(fromBackend))
	if _, err := io.ReadFull(client, gotBackend); err != nil || string(gotBackend) != string(fromBackend) {
		t.Fatalf("client received %q, %v", gotBackend, err)
	}
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Lite pipe did not finish after client close")
	}

	session.SetKind(connectiontelemetry.Gameplay)
	session.Observe(ctx, connectiontelemetry.Play, connectiontelemetry.Success)
	// Reclassification flushes bytes under the previous accepted/unknown
	// snapshot; the new gameplay event deliberately starts at zero.
	event := collector.events[len(collector.events)-2]
	if event.BytesRead != int64(len(fromClient)) || event.BytesWritten != int64(len(fromBackend)) {
		t.Fatalf("Lite io.Copy bytes = rx=%d tx=%d, want %d/%d", event.BytesRead, event.BytesWritten, len(fromClient), len(fromBackend))
	}
}

func TestPipePreservesTCPHalfCloseAndJoinsBothDirections(t *testing.T) {
	client, gateClient := tcpPair(t)
	gateBackend, backend := tcpPair(t)
	t.Cleanup(func() { _ = client.Close(); _ = gateClient.Close(); _ = gateBackend.Close(); _ = backend.Close() })

	done := make(chan struct{})
	go func() { pipe(logr.Discard(), gateClient, gateBackend); close(done) }()
	if _, err := client.Write([]byte("request")); err != nil {
		t.Fatal(err)
	}
	if err := client.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	request, err := io.ReadAll(backend)
	if err != nil || string(request) != "request" {
		t.Fatalf("backend request = %q, %v", request, err)
	}
	if _, err := backend.Write([]byte("response")); err != nil {
		t.Fatal(err)
	}
	if err := backend.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	response, err := io.ReadAll(client)
	if err != nil || string(response) != "response" {
		t.Fatalf("client response = %q, %v", response, err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pipe did not join both TCP copy workers")
	}
}

func tcpPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	dialed, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case server := <-accepted:
		return dialed, server
	case <-time.After(time.Second):
		_ = dialed.Close()
		t.Fatal("TCP pair accept timed out")
		return nil, nil
	}
}

type liteTelemetryCollector struct{ events []connectiontelemetry.Event }

func newLiteTelemetryCollector() *liteTelemetryCollector { return &liteTelemetryCollector{} }
func (c *liteTelemetryCollector) Observe(_ context.Context, event connectiontelemetry.Event) {
	c.events = append(c.events, event)
}
func (c *liteTelemetryCollector) last(t *testing.T) connectiontelemetry.Event {
	t.Helper()
	if len(c.events) == 0 {
		t.Fatal("no telemetry events")
	}
	return c.events[len(c.events)-1]
}
