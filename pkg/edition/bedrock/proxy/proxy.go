package proxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/errs"
)

// Options are the options for a new Bedrock edition Proxy.
type Options struct {
	// Config requires a valid configuration.
	Config *config.Config
	// The event manager to use.
	// If none is set, no events are sent.
	EventMgr event.Manager
	// Logger is the logger to be used by the Proxy.
	// If none is set, does no logging at all.
	Logger logr.Logger
}

// New takes a config that should have been validated by
// config.Validate and returns a new initialized Proxy ready to start.
func New(options Options) (p *Proxy, err error) {
	if options.Config == nil {
		return nil, errs.ErrMissingConfig
	}
	eventMgr := options.EventMgr
	if eventMgr == nil {
		eventMgr = event.Nop
	}

	p = &Proxy{
		event:  eventMgr,
		log:    logr.Discard(),
		config: options.Config,
	}

	p.listenerKey, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("error generating public/private key: %w", err)
	}

	return p, nil
}

// Proxy is Gate's Bedrock edition Minecraft proxy.
type Proxy struct {
	log    logr.Logger
	event  event.Manager
	config *config.Config

	startTime atomic.Value

	closeMu       sync.Mutex
	closeListener chan struct{}
	started       bool

	listenerKey       *ecdsa.PrivateKey
	geyserIntegration *geyser.Integration
	javaProxy         interface{} // Reference to Java proxy for integration
}

// SetJavaProxy sets the Java proxy reference for integration.
func (p *Proxy) SetJavaProxy(javaProxy interface{}) {
	p.javaProxy = javaProxy
}

func (p *Proxy) Start(ctx context.Context) error {
	p.log = logr.FromContextOrDiscard(ctx)

	// Initialize Geyser integration if Java proxy is available
	if p.javaProxy != nil {
		if javaProxy, ok := p.javaProxy.(*jproxy.Proxy); ok {
			integration, err := geyser.NewIntegration(ctx, javaProxy, p.config)
			if err != nil {
				p.log.Error(err, "failed to initialize geyser integration")
				return err
			}

			p.geyserIntegration = integration

			if err := integration.Start(); err != nil {
				p.log.Error(err, "failed to start geyser integration")
				return err
			}
		}
	}

	p.log.Info("bedrock proxy started with geyser integration")

	// Wait for context cancellation
	<-ctx.Done()
	return nil
}
