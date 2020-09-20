package proxy

import (
	"go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/event"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/runtime/manager"
	"go.minekube.com/gate/pkg/util/errs"
)

// Proxy is Gate's Bedrock edition Minecraft proxy.
type Proxy struct {
	log   logr.Logger
	event event.Manager
}

// Options are the options for a new Bedrock edition Proxy.
type Options struct {
	// Config requires a valid configuration.
	Config *config.Config
	// Logger is the logger to be used by the Proxy.
	// If none is set, the managers logger is used.
	Logger logr.Logger
}

// New takes a config that should have been validated by
// config.Validate and returns a new initialized Proxy ready to start.
func New(mgr manager.Manager, options Options) (*Proxy, error) {
	if options.Config == nil {
		return nil, errs.ErrMissingConfig
	}
	log := options.Logger
	if log == nil {
		log = mgr.Logger().WithName("bedrock-proxy")
	}

	p := &Proxy{
		event: mgr.Event(),
		log:   log,
	}
	return p, mgr.Add(p)
}

func (p *Proxy) Start(stop <-chan struct{}) error {
	return nil
}
