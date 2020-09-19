package proxy

import (
	"go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/event"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/runtime/manager"
)

// Proxy is Gate's Bedrock edition Minecraft proxy.
type Proxy struct {
	log   logr.Logger
	event event.Manager
}

func New(mgr manager.Manager, config config.Config) (*Proxy, error) {
	p := &Proxy{
		event: mgr.Event(),
		log:   mgr.Logger(),
	}
	return p, mgr.Add(p)
}

func (p *Proxy) Start(stop <-chan struct{}) error {
	return nil
}
