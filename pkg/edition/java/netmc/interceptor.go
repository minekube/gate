package netmc

import (
"context"
"fmt"

"github.com/davecgh/go-spew/spew"
"github.com/go-logr/logr"
"go.opentelemetry.io/otel"
"go.opentelemetry.io/otel/attribute"
"go.opentelemetry.io/otel/trace"
"go.minekube.com/gate/pkg/gate/proto"
)

// PacketInterceptor intercepts packets for telemetry and statistics
type PacketInterceptor interface {
	// InterceptPacket intercepts a packet before it's handled
	InterceptPacket(ctx context.Context, pc *proto.PacketContext) error
}

// TracedPacketContext wraps a packet context with source information
type TracedPacketContext struct {
	*proto.PacketContext
	Conn        *minecraftConn // Source connection
	Interceptor PacketInterceptor
}

// telemetryInterceptor implements PacketInterceptor for OpenTelemetry
type telemetryInterceptor struct {
	log    logr.Logger
	tracer trace.Tracer
}

// NewTelemetryInterceptor creates a new telemetry interceptor
func NewTelemetryInterceptor(log logr.Logger) PacketInterceptor {
	return &telemetryInterceptor{
		log:    log,
		tracer: otel.Tracer("netmc"),
	}
}

// InterceptPacket implements PacketInterceptor
func (t *telemetryInterceptor) InterceptPacket(ctx context.Context, pc *proto.PacketContext) error {
	if pc == nil {
		return nil
	}

	// Create span for packet handling
	ctx, span := t.tracer.Start(ctx, "HandlePacket",
		trace.WithAttributes(
			attribute.String("packet.id", pc.PacketID.String()),
			attribute.Int("packet.size", pc.Size),
			attribute.String("packet.direction", pc.Direction.String()),
		))
	defer span.End()

	// Add packet type info and dump if known packet and debug enabled
	if pc.KnownPacket() {
	attrs := []attribute.KeyValue{
	attribute.String("packet.type", fmt.Sprintf("%T", pc.Packet)),
	}
	
	// Add detailed packet dump in debug mode
	if t.log.V(1).Enabled() {
	attrs = append(attrs,
	attribute.String("packet.dump", spew.Sdump(pc.Packet)),
	)
	}
	
	span.SetAttributes(attrs...)
	}

	return nil
}