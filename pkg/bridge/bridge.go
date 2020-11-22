// Package bridge contains packages for setting up a bridge between proxies of different Minecraft editions.
// Refer to Bridge struct.
package bridge

import (
	"errors"
	"fmt"
	"go.minekube.com/gate/pkg/edition"
	bproxy "go.minekube.com/gate/pkg/edition/bedrock/proxy"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/logr"
	"sync"
)

// Bridge allows "cross-play" between different Minecraft edition (Bedrock <-> Java) proxies.
// Exposed fields in this struct should be set before calling Setup.
//
// It does so by registering various handlers and interceptors to the given proxies
// to translate connection protocols.
//
// This struct may only be useful until Setup was called and can get garbage collected afterwards.
type Bridge struct {
	Log logr.Logger // The logger used in bridging-code.

	// At least two editions must be set.
	JavaProxy    *jproxy.Proxy // Holds java edition players.
	BedrockProxy *bproxy.Proxy // Holds bedrock edition players

	setupOnce sync.Once
	err       error
}

// Setup sets up the bridge between the given proxies.
func (b *Bridge) Setup() (err error) {
	if b == nil {
		return nil
	}
	b.setupOnce.Do(func() { b.err = b.setup() })
	return b.err
}

func (b *Bridge) valid() error {
	if b.BedrockProxy == nil && b.JavaProxy == nil {
		return fmt.Errorf("proxy must run at least one edition (%s and/or %s)",
			edition.Java, edition.Bedrock)
	}
	if b.Log == nil {
		b.Log = logr.Log.WithName("bridge")
	}
	return nil
}

func (b *Bridge) setup() (err error) {
	if err := b.valid(); err != nil {
		return fmt.Errorf("invalid bridge setup: %v", err)
	}

	// TODO setup bedrock <---> java edition bridges by registering:
	//  - packet interceptors
	//  - event subscribers
	return errors.New("bridge features not implemented yet")
}
