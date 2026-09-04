package proxy

import (
	"context"
	"fmt"
	"reflect"
	"sync"

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
		log:       options.Logger,
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

	geyserIntegration *geyser.Integration
	javaProxy         *jproxy.Proxy // Reference to Java proxy for integration
	mu                sync.RWMutex
	reloadMu          sync.Mutex
	runtimeFailures   chan error
	reloadGeneration  uint64
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

	p.mu.Lock()
	p.geyserIntegration = integration
	p.runtimeFailures = make(chan error, 1)
	runtimeFailures := p.runtimeFailures
	p.mu.Unlock()

	if err := integration.Start(); err != nil {
		p.log.Error(err, "failed to start geyser integration")
		integration.Stop()
		p.mu.Lock()
		p.geyserIntegration = nil
		p.mu.Unlock()
		return err
	}
	p.watchRuntime(integration)

	// Listen for config reloads and restart Geyser integration when relevant fields change
	unsubReload := reload.Subscribe(p.event, func(e *bedrockConfigUpdateEvent) {
		p.handleConfigUpdate(ctx, e)
	})

	p.log.Info("bedrock proxy started with geyser integration")

	defer func() {
		if unsubReload != nil {
			unsubReload()
		}
		p.stopIntegration()
		p.log.Info("bedrock proxy stopped")
	}()

	// A managed GeyserLite process can exit after it has passed its startup
	// health check. Returning the failure stops Gate as a whole, which in turn
	// makes the Moxy/Fly runtime check fail instead of leaving Java/TCP green
	// while Bedrock UDP has disappeared.
	select {
	case <-ctx.Done():
		return nil
	case err := <-runtimeFailures:
		return fmt.Errorf("bedrock runtime failed: %w", err)
	}
}

func (p *Proxy) watchRuntime(integration *geyser.Integration) {
	p.watchRuntimeSignals(integration.RuntimeErrors(), integration.Done(), func(err error) {
		p.reportIntegrationRuntimeFailure(integration, err)
	})
}

func (p *Proxy) watchRuntimeSignals(runtimeErrors <-chan error, done <-chan struct{}, report func(error)) <-chan struct{} {
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-done:
			return
		case err := <-runtimeErrors:
			if err == nil {
				return
			}
			report(err)
		}
	}()
	return stopped
}

func (p *Proxy) stopIntegration() {
	p.mu.Lock()
	integration := p.geyserIntegration
	p.geyserIntegration = nil
	p.mu.Unlock()
	if integration != nil {
		integration.Stop()
	}
}

// beginReload first makes the old integration non-current before stopping it.
// A late failure from that old instance can therefore never abort a healthy
// replacement. The generation also prevents a slower failed reload from
// reporting after a newer configuration has superseded it.
func (p *Proxy) beginReload() uint64 {
	p.mu.Lock()
	p.reloadGeneration++
	generation := p.reloadGeneration
	integration := p.geyserIntegration
	p.geyserIntegration = nil
	p.mu.Unlock()
	if integration != nil {
		integration.Stop()
	}
	return generation
}

func (p *Proxy) reportReloadFailure(generation uint64, err error) {
	if err == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reloadGeneration != generation || p.runtimeFailures == nil {
		return
	}
	select {
	case p.runtimeFailures <- err:
	default:
	}
}

func (p *Proxy) reportIntegrationRuntimeFailure(integration *geyser.Integration, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.geyserIntegration != integration || p.runtimeFailures == nil {
		return
	}
	select {
	case p.runtimeFailures <- err:
	default:
	}
}

func (p *Proxy) handleConfigUpdate(ctx context.Context, e *bedrockConfigUpdateEvent) {
	// Config events can arrive back-to-back while a managed runtime is still
	// starting. Serialize replacement so a newer event cannot leave the old
	// listener stopped while an earlier replacement is still in flight.
	p.reloadMu.Lock()
	defer p.reloadMu.Unlock()

	if e == nil {
		return
	}
	prev := e.PrevConfig
	curr := e.Config
	if curr == nil {
		p.beginReload()
		return
	}

	if prev == nil || requiresRestart(prev, curr) {
		p.log.Info("restarting geyser integration due to bedrock config change")
		generation := p.beginReload()
		p.config = curr
		integ, err := geyser.NewIntegration(ctx, p.javaProxy, p.config)
		if err != nil {
			p.log.Error(err, "failed to re-initialize geyser integration")
			p.reportReloadFailure(generation, fmt.Errorf("failed to re-initialize geyser integration: %w", err))
			return
		}
		if err := integ.Start(); err != nil {
			p.log.Error(err, "failed to restart geyser integration")
			integ.Stop()
			p.reportReloadFailure(generation, fmt.Errorf("failed to restart geyser integration: %w", err))
			return
		}
		p.mu.Lock()
		current := p.reloadGeneration == generation
		if current {
			p.geyserIntegration = integ
		}
		p.mu.Unlock()
		if !current {
			integ.Stop()
			return
		}
		p.watchRuntime(integ)
		p.log.Info("geyser integration reloaded")
		return
	}

	p.config = curr
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
	if !reflect.DeepEqual(prev.BackendFloodgate, curr.BackendFloodgate) {
		return true
	}

	// Check managed Geyser settings
	prevManaged := prev.GetManaged()
	currManaged := curr.GetManaged()

	if prevManaged.Enabled != currManaged.Enabled ||
		prevManaged.Engine != currManaged.Engine ||
		prevManaged.Mode != currManaged.Mode ||
		prevManaged.JarURL != currManaged.JarURL ||
		prevManaged.JavaPath != currManaged.JavaPath ||
		prevManaged.LibraryPath != currManaged.LibraryPath ||
		prevManaged.BinaryPath != currManaged.BinaryPath ||
		prevManaged.Mirror != currManaged.Mirror ||
		prevManaged.Version != currManaged.Version ||
		prevManaged.Offline != currManaged.Offline ||
		!reflect.DeepEqual(prevManaged.ExtraArgs, currManaged.ExtraArgs) {
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
