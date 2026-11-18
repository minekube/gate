package proxy

import (
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/gate/proto"
)

// rewriteServerBrand resolves the brand Gate sends to the client for a brand
// message a backend server sent: the backend's brand with config.brandSuffix
// appended, as overridden per session by any ServerBrandRewriteEvent handler.
//
// The returned message is the original one when the resolved brand equals the
// backend's brand, so both a suppressed rewrite and an empty configured suffix
// forward the backend's bytes untouched (an empty backend brand therefore adds
// nothing to the session). Only a resolved brand that differs is re-framed for
// the client's protocol.
func (p *Proxy) rewriteServerBrand(
	player Player,
	message *plugin.Message,
	protocol proto.Protocol,
) *plugin.Message {
	if message == nil || !plugin.McBrand(message) {
		return message
	}

	originalBrand := plugin.ReadBrandMessage(message.Data)
	event := &ServerBrandRewriteEvent{
		player:        player,
		originalBrand: originalBrand,
		brand:         originalBrand + p.brandSuffix(),
	}
	p.fireServerBrandRewrite(event)

	if event.brand == originalBrand {
		return message // no rewrite: forward the backend's brand as-is
	}
	return plugin.RewriteMinecraftBrandTo(message, protocol, event.brand)
}

// fireServerBrandRewrite fires the event to the proxy's event handlers and waits
// for them, so a handler's SetBrand is visible before the brand is written.
func (p *Proxy) fireServerBrandRewrite(event *ServerBrandRewriteEvent) {
	if p.event == nil {
		return
	}
	p.event.Fire(event)
}

// brandSuffix returns the configured brand suffix, falling back to the wire
// layer's default when the proxy has no config (unit-test handlers) or the
// config does not set one.
func (p *Proxy) brandSuffix() string {
	if cfg := p.config(); cfg != nil {
		return cfg.ResolveBrandSuffix()
	}
	return plugin.DefaultBrandSuffix
}
