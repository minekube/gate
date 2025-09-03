package proxy

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/internal/reload"
	"go.minekube.com/gate/pkg/util/errs"
)

// Options are the options for a new Bedrock edition Proxy.
type Options struct {
	// Config requires a valid configuration.
	Config *config.Config
	// JavaProxy is required for integrating Geyser with the Java proxy.
	JavaProxy *jproxy.Proxy
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
	if options.JavaProxy == nil {
		return nil, fmt.Errorf("java proxy is required for bedrock geyser integration")
	}
	eventMgr := options.EventMgr
	if eventMgr == nil {
		eventMgr = event.Nop
	}

	p = &Proxy{
		event:     eventMgr,
		log:       logr.Discard(),
		config:    options.Config,
		javaProxy: options.JavaProxy,
	}

	return p, nil
}

// Proxy is Gate's Bedrock edition Minecraft proxy.
type Proxy struct {
	log    logr.Logger
	event  event.Manager
	config *config.Config

	startTime atomic.Value

	geyserIntegration *geyser.Integration
	javaProxy         *jproxy.Proxy // Reference to Java proxy for integration
}

func (p *Proxy) Event() event.Manager { return p.event }

func (p *Proxy) Start(ctx context.Context) error {
	p.log = logr.FromContextOrDiscard(ctx)

	// Initialize Geyser integration
	integration, err := geyser.NewIntegration(ctx, p.javaProxy, p.config)
	if err != nil {
		p.log.Error(err, "failed to initialize geyser integration")
		return err
	}

	p.geyserIntegration = integration

	if err := integration.Start(); err != nil {
		p.log.Error(err, "failed to start geyser integration")
		return err
	}

	// Listen for config reloads and restart Geyser integration when relevant fields change
	unsubReload := reload.Subscribe(p.event, func(e *bedrockConfigUpdateEvent) {
		prev := e.PrevConfig
		curr := e.Config
		// Replace config for future use
		*p.config = *curr

		// Check if restart is required
		if requiresRestart(prev, curr) {
			p.log.Info("restarting geyser integration due to bedrock config change")
			if p.geyserIntegration != nil {
				p.geyserIntegration.Stop()
			}
			integ, err := geyser.NewIntegration(ctx, p.javaProxy, p.config)
			if err != nil {
				p.log.Error(err, "failed to re-initialize geyser integration")
				return
			}
			p.geyserIntegration = integ
			if err := integ.Start(); err != nil {
				p.log.Error(err, "failed to restart geyser integration")
				return
			}
			p.log.Info("geyser integration reloaded")
		}
	})

	p.log.Info("bedrock proxy started with geyser integration")

	// Block until context cancellation - cleanup on exit
	<-ctx.Done()

	// Cleanup
	if unsubReload != nil {
		unsubReload()
	}
	if p.geyserIntegration != nil {
		p.geyserIntegration.Stop()
	}

	p.log.Info("bedrock proxy stopped")
	return nil
}

// requiresRestart determines if a Geyser integration restart is needed based on config changes.
// Returns true if any critical configuration has changed that requires restarting Geyser.
func requiresRestart(prev, curr *config.Config) bool {
	// Check basic connection and authentication settings
	if prev.GeyserListenAddr != curr.GeyserListenAddr ||
		prev.UsernameFormat != curr.UsernameFormat ||
		prev.FloodgateKeyPath != curr.FloodgateKeyPath {
		return true
	}

	// Check managed Geyser settings
	prevManaged := prev.GetManaged()
	currManaged := curr.GetManaged()

	if prevManaged.Enabled != currManaged.Enabled ||
		prevManaged.JarURL != currManaged.JarURL {
		return true
	}

	// Check for any changes in config overrides (including bedrock port, debug settings, etc.)
	if !reflect.DeepEqual(prevManaged.ConfigOverrides, currManaged.ConfigOverrides) {
		return true
	}

	// No critical changes detected
	return false
}

type bedrockConfigUpdateEvent = reload.ConfigUpdateEvent[config.Config]
