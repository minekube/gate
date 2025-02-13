package telemetry

import (
	"context"
	"fmt"
	"net"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

// TracedServerConnection wraps a proxy server connection with tracing
type TracedServerConnection struct {
	proxy.ServerConnection
	tracer trace.Tracer
}

// WithServerConnectionTracing wraps server connection functions with tracing
func WithServerConnectionTracing(s proxy.ServerConnection) proxy.ServerConnection {
	if s == nil {
		return nil
	}

	ts := &TracedServerConnection{
		ServerConnection: s,
		tracer:          tracer,
	}

	// Start connection span
	ctx := context.Background()
	_, span := ts.tracer.Start(ctx, "server.connection",
		trace.WithAttributes(
			semconv.PeerServiceKey.String("minecraft-server"),
			attribute.String("connection_type", "server"),
		))
	defer span.End()

	return ts
}

// WithConnectionTracing wraps a net.Conn with OpenTelemetry tracing
func WithConnectionTracing(conn net.Conn, name string) net.Conn {
	if conn == nil {
		return nil
	}
	return &tracedConn{
		Conn: conn,
		name: name,
	}
}

type tracedConn struct {
	net.Conn
	name string
}

func (t *tracedConn) Read(b []byte) (n int, err error) {
	_, span := tracer.Start(context.Background(), fmt.Sprintf("%s.read", t.name))
	defer span.End()

	n, err = t.Conn.Read(b)
	span.SetAttributes(
		attribute.Int("bytes_read", n),
	)
	return n, err
}

func (t *tracedConn) Write(b []byte) (n int, err error) {
	_, span := tracer.Start(context.Background(), fmt.Sprintf("%s.write", t.name))
	defer span.End()

	n, err = t.Conn.Write(b)
	span.SetAttributes(
		attribute.Int("bytes_written", n),
	)
	return n, err
}

func (t *tracedConn) Close() error {
	_, span := tracer.Start(context.Background(), fmt.Sprintf("%s.close", t.name))
	defer span.End()

	return t.Conn.Close()
}