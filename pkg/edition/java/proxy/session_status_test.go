package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/gate/proto"
)

const statusLogStressIterations = 10_000

type statusLogSink struct {
	mu    sync.Mutex
	lines []string
}

func (s *statusLogSink) logger(verbosity int) logr.Logger {
	return funcr.New(func(prefix, args string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.lines = append(s.lines, prefix+" "+args)
	}, funcr.Options{Verbosity: verbosity})
}

func (s *statusLogSink) count(message string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, line := range s.lines {
		if strings.Contains(line, `"msg"="`+message+`"`) {
			count++
		}
	}
	return count
}

type statusLogTestConn struct {
	netmc.MinecraftConn
	writeErr error
	writes   int
	closes   int
}

func (c *statusLogTestConn) WritePacket(proto.Packet) error {
	c.writes++
	return c.writeErr
}

func (c *statusLogTestConn) Close() error {
	c.closes++
	return nil
}

type inactiveStatusLogInbound struct{ Inbound }

func (*inactiveStatusLogInbound) Active() bool { return false }

func TestStatusResponseExpectedWriteFailuresAreDebug(t *testing.T) {
	tests := map[string]error{
		"closed connection": netmc.ErrClosedConn,
		"closed pipe":       io.ErrClosedPipe,
		"closed network":    net.ErrClosed,
		"connection reset": &net.OpError{
			Op:  "write",
			Err: fmt.Errorf("wrapped reset: %w", syscall.ECONNRESET),
		},
		"broken pipe": &net.OpError{
			Op:  "write",
			Err: fmt.Errorf("wrapped broken pipe: %w", syscall.EPIPE),
		},
		"network timeout": fmt.Errorf("wrapped timeout: %w", &net.DNSError{
			Err:       "status response timed out",
			IsTimeout: true,
		}),
	}

	for name, writeErr := range tests {
		t.Run(name, func(t *testing.T) {
			infoSink := new(statusLogSink)
			debugSink := new(statusLogSink)
			h := &statusSessionHandler{conn: &statusLogTestConn{writeErr: writeErr}}

			for range statusLogStressIterations {
				h.writeStatusResponse(infoSink.logger(0), &packet.StatusResponse{Status: "{}"})
				h.writeStatusResponse(debugSink.logger(1), &packet.StatusResponse{Status: "{}"})
			}

			if got := infoSink.count("error writing status response"); got != 0 {
				t.Fatalf("default-level records = %d, want 0", got)
			}
			if got := debugSink.count("error writing status response"); got != statusLogStressIterations {
				t.Fatalf("debug records = %d, want %d", got, statusLogStressIterations)
			}
		})
	}
}

func TestInactiveStatusResponsesAreDebug(t *testing.T) {
	infoSink := new(statusLogSink)
	debugSink := new(statusLogSink)

	for range statusLogStressIterations {
		newInactiveStatusHandler(t, infoSink.logger(0)).handleStatusRequest(&proto.PacketContext{
			Packet: &packet.StatusRequest{},
		})
		newInactiveStatusHandler(t, debugSink.logger(1)).handleStatusRequest(&proto.PacketContext{
			Packet: &packet.StatusRequest{},
		})
	}

	if got := infoSink.count("status response not sent because inbound is inactive"); got != 0 {
		t.Fatalf("default-level records = %d, want 0", got)
	}
	if got := debugSink.count("status response not sent because inbound is inactive"); got != statusLogStressIterations {
		t.Fatalf("debug records = %d, want %d", got, statusLogStressIterations)
	}
}

func TestStatusResponseStructuralWriteFailureRemainsInfo(t *testing.T) {
	sink := new(statusLogSink)
	conn := &statusLogTestConn{writeErr: errors.New("packet encode failed")}
	h := &statusSessionHandler{conn: conn}

	h.writeStatusResponse(sink.logger(0), &packet.StatusResponse{Status: "{}"})

	if got := sink.count("error writing status response"); got != 1 {
		t.Fatalf("default-level records = %d, want 1", got)
	}
	if conn.writes != 1 {
		t.Fatalf("writes = %d, want 1", conn.writes)
	}
}

func TestStatusResponseClosedPeerIsDebug(t *testing.T) {
	testLogCount := func(t *testing.T, verbosity int) int {
		t.Helper()

		server, client := net.Pipe()
		ctx := logr.NewContext(context.Background(), logr.Discard())
		conn, _ := netmc.NewMinecraftConn(ctx, server, proto.ServerBound, time.Second, time.Second, -1, nil)
		conn.SetState(state.Status)
		t.Cleanup(func() { _ = conn.Close() })
		_ = client.Close()

		sink := new(statusLogSink)
		h := &statusSessionHandler{conn: conn}
		h.writeStatusResponse(sink.logger(verbosity), &packet.StatusResponse{Status: "{}"})
		return sink.count("error writing status response")
	}

	if got := testLogCount(t, 0); got != 0 {
		t.Fatalf("default-level records = %d, want 0", got)
	}
	if got := testLogCount(t, 1); got != 1 {
		t.Fatalf("debug records = %d, want 1", got)
	}
}

func TestDuplicateStatusRequestsDoNotMultiplyDiagnostics(t *testing.T) {
	sink := new(statusLogSink)
	h := newInactiveStatusHandler(t, sink.logger(1))
	conn := h.conn.(*statusLogTestConn)
	request := &proto.PacketContext{Packet: &packet.StatusRequest{}}

	h.handleStatusRequest(request)
	h.handleStatusRequest(request)

	if got := sink.count("status response not sent because inbound is inactive"); got != 1 {
		t.Fatalf("inactive diagnostics = %d, want 1", got)
	}
	if conn.writes != 0 {
		t.Fatalf("writes = %d, want 0", conn.writes)
	}
	if conn.closes != 1 {
		t.Fatalf("closes after duplicate = %d, want 1", conn.closes)
	}
}

func newInactiveStatusHandler(t *testing.T, log logr.Logger) *statusSessionHandler {
	t.Helper()

	eventMgr := event.New()
	event.Subscribe(eventMgr, 0, func(*PingEvent) {})
	return &statusSessionHandler{
		sessionHandlerDeps: &sessionHandlerDeps{eventMgr: eventMgr},
		conn:               &statusLogTestConn{},
		inbound:            &inactiveStatusLogInbound{},
		log:                log,
		resolvePingResponse: func(log logr.Logger, _ *proto.PacketContext) (logr.Logger, *packet.StatusResponse, error) {
			return log, &packet.StatusResponse{Status: "{}"}, nil
		},
	}
}
