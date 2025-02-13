package telemetry

import (
	"context"
	"fmt"

	"github.com/robinbraemer/event"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

// ProxyServer represents a common interface for proxy servers
type ProxyServer interface {
	Event() event.Manager
}

// InstrumentProxy adds OpenTelemetry instrumentation to key proxy functions
func (t *Telemetry) InstrumentProxy(p ProxyServer) {
	if p == nil {
		return
	}

	// Subscribe to events
	t.subscribeEvents(p)
}

func (t *Telemetry) subscribeEvents(p ProxyServer) {
	eventMgr := p.Event()
	if eventMgr == nil {
		return
	}

	// Track player login
	eventMgr.Subscribe(&proxy.LoginEvent{}, 0, event.HandlerFunc(func(e event.Event) {
		loginEvent := e.(*proxy.LoginEvent)
		_, span := t.tracer.Start(context.Background(), "player.Login",
			trace.WithAttributes(
				attribute.String("username", loginEvent.Player().Username()),
				attribute.String("uuid", loginEvent.Player().ID().String()),
				attribute.Bool("online_mode", loginEvent.Player().OnlineMode()),
			))
		defer span.End()

		// Record metric
		t.RecordPlayerConnection(context.Background(), loginEvent.Player().Username())
		t.UpdatePlayerCount(context.Background(), 1)
	}))

	// Track player disconnect
	eventMgr.Subscribe(&proxy.DisconnectEvent{}, 0, event.HandlerFunc(func(e event.Event) {
		disconnectEvent := e.(*proxy.DisconnectEvent)
		_, span := t.tracer.Start(context.Background(), "player.Disconnect",
			trace.WithAttributes(
				attribute.String("username", disconnectEvent.Player().Username()),
				attribute.String("uuid", disconnectEvent.Player().ID().String()),
			))
		defer span.End()

		// Record metrics
		t.UpdatePlayerCount(context.Background(), -1)
	}))

	// Track server connections
	eventMgr.Subscribe(&proxy.ServerPreConnectEvent{}, 0, event.HandlerFunc(func(e event.Event) {
		serverEvent := e.(*proxy.ServerPreConnectEvent)
		if serverEvent.Server() == nil {
			return
		}
		_, span := t.tracer.Start(context.Background(), "player.ServerConnect",
			trace.WithAttributes(
				attribute.String("username", serverEvent.Player().Username()),
				attribute.String("server", serverEvent.Server().ServerInfo().Name()),
			))
		defer span.End()
	}))

	// Track command executions
	eventMgr.Subscribe(&proxy.CommandExecuteEvent{}, 0, event.HandlerFunc(func(e event.Event) {
		cmdEvent := e.(*proxy.CommandExecuteEvent)
		_, span := t.tracer.Start(context.Background(), "command.Execute",
			trace.WithAttributes(
				attribute.String("command", cmdEvent.Command()),
				attribute.String("source", fmt.Sprintf("%T", cmdEvent.Source())),
			))
		defer span.End()
	}))

	// Track plugin messages
	eventMgr.Subscribe(&proxy.PluginMessageEvent{}, 0, event.HandlerFunc(func(e event.Event) {
		pluginEvent := e.(*proxy.PluginMessageEvent)
		_, span := t.tracer.Start(context.Background(), "plugin.Message",
			trace.WithAttributes(
				attribute.String("identifier", fmt.Sprintf("%v", pluginEvent.Identifier())),
				attribute.Int("data_length", len(pluginEvent.Data())),
				attribute.String("source", fmt.Sprintf("%T", pluginEvent.Source())),
			))
		defer span.End()
	}))
}