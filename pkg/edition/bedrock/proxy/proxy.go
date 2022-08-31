package proxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	"github.com/sandertv/go-raknet"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/runtime/event"
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

	listenerKey *ecdsa.PrivateKey
}

func (p *Proxy) Start(ctx context.Context) error {
	<-ctx.Done()
	p.log = logr.FromContextOrDiscard(ctx)
	// TODO
	return nil
}

func (p *Proxy) listenAndServe(addr string, stop <-chan struct{}) error {
	select {
	case <-stop:
		return nil
	default:
	}

	ln, err := raknet.Listen(addr)
	if err != nil {
		return err
	}
	// TODO the raknet library sadly strictly couples the listener and accepted connections,
	// make sure we first send players a disconnect packet before closing the listener
	defer ln.Close()
	go func() { <-stop; _ = ln.Close() }()

	p.log.Info("Listening for connections", "addr", addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			// TODO
			return fmt.Errorf("error accepting new connection: %w", err)
		}
		go p.handleRawConn(conn)
	}
}

func (p *Proxy) handleRawConn(raw net.Conn) {
	defer raw.Close()
}
