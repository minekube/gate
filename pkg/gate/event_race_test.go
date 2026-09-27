package gate

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/configs"
	"go.minekube.com/gate/pkg/gate/config"
	"go.minekube.com/gate/pkg/internal/reload"
	"go.minekube.com/gate/pkg/runtime/process"
	connectcfg "go.minekube.com/gate/pkg/util/connectutil/config"
)

// This test covers the event-manager defect Gate is exposed to: whenever the
// API is enabled Gate has two subscribers of the config-update event
// (pkg/gate/connect.go and pkg/gate/api.go) and both unsubscribe in a defer
// when their runnable returns, so a config update delivered while one of them
// is unsubscribing overlaps an unsubscribe of a sibling subscriber of the same
// event type. The event manager Gate depends on (robinbraemer/event v0.1.1)
// rewrites that subscriber slice in place, so the delivery in flight reads a
// zeroed slot, calls a nil subscriber and — with the default panic recovery —
// silently drops the rest of the delivery.
//
// pkg/util/eventmgr removes that class of defect by giving the event manager a
// single subscriber per event type; this test proves the manager Gate builds
// delivers an in-flight config update to every subscriber that was subscribed
// when it started, including the one that unsubscribed mid-delivery, and that
// Gate's own subscriber still handled it.

// TestConfigUpdateInFlightWithSiblingUnsubscribeIsNotLost forces the
// interleaving the defect needs — a config update held inside its first
// subscriber while a sibling subscriber of the same event type unsubscribes —
// and asserts the delivery is not lost. Without the event manager's mitigation
// the delivery panics here (nil subscriber) and the sibling is skipped.
//
// The interleaving is forced, not raced: the sibling only unsubscribes after
// the held subscriber reported that the delivery started, and the held
// subscriber only returns afterwards. No sleeps.
func TestConfigUpdateInFlightWithSiblingUnsubscribeIsNotLost(t *testing.T) {
	var (
		created atomic.Int32
		steps   stepLog
	)
	stubConnectRuntime(t, func(connectcfg.Config, connectcfg.Instance) (process.Runnable, error) {
		n := int(created.Add(1))
		steps.add(fmt.Sprintf("create %d", n))
		return process.RunnableFunc(func(ctx context.Context) error {
			steps.add(fmt.Sprintf("start %d", n))
			<-ctx.Done()
			steps.add(fmt.Sprintf("stopped %d", n))
			return nil
		}), nil
	})

	cfg := loadTestConfig(t, configs.MinimalConfigBytes)
	cfg.Config.Bind = reserveAddr(t)
	cfg.Connect.Enabled = true
	cfg.Connect.Name = "gate-inflight-unsubscribe-a"

	// The manager Gate is handed, as pkg/gate.Start builds it. Recovery is
	// disabled so a delivery that reads a subscriber the event manager's
	// unsubscribe zeroed panics here instead of being swallowed (which is what
	// makes the defect silent in production).
	events := event.New(event.WithRecoverPanic(false))

	run := startGate(t, cfg, events)

	// Gate's Connect runnable subscribes to the config update before it starts
	// its runtime, so this proves Gate's own subscriber is registered.
	steps.waitFor(t, "start 1", startupTimeout)

	// The manager the proxies fire and subscribe on, i.e. the one Gate wired
	// into its components.
	mgr := run.javaProxy.Event()

	var (
		held       atomic.Int32
		sibling    atomic.Int32
		inFlight   = make(chan struct{})
		inFlightOn = sync.OnceFunc(func() { close(inFlight) })
		release    = make(chan struct{})
		releaseOn  = sync.OnceFunc(func() { close(release) })
	)
	defer releaseOn()

	// The held subscriber: higher priority than Gate's own subscribers, so it
	// is called first and holds the delivery while the sibling unsubscribes.
	event.Subscribe(mgr, 1, func(*reload.ConfigUpdateEvent[config.Config]) {
		held.Add(1)
		inFlightOn()
		<-release
	})
	// The sibling: same event type, registered after Gate's own subscribers,
	// unsubscribed while the delivery is in flight.
	unsubscribeSibling := event.Subscribe(mgr, 0, func(*reload.ConfigUpdateEvent[config.Config]) {
		sibling.Add(1)
	})

	updated := *cfg
	updated.Connect.Name = "gate-inflight-unsubscribe-b"

	var panicked atomic.Value // the panic the delivery recovered, nil if none
	fired := make(chan struct{})
	go func() {
		defer close(fired)
		defer func() {
			if r := recover(); r != nil {
				panicked.Store(r)
			}
		}()
		reload.FireConfigUpdate(mgr, &updated, cfg)
	}()

	select {
	case <-inFlight:
	case <-time.After(startupTimeout):
		t.Fatal("the config update was never delivered to its held subscriber")
	}

	// The delivery is in flight right now: this unsubscribe overlaps it.
	unsubscribeSibling()
	releaseOn()

	select {
	case <-fired:
	case <-time.After(shutdownTimeout):
		t.Fatal("the in-flight config update never returned")
	}

	if p := panicked.Load(); p != nil {
		t.Fatalf("the in-flight config update panicked (%v): the delivery read a "+
			"subscriber that the concurrent unsubscribe zeroed", p)
	}
	require.Equal(t, int32(1), held.Load(),
		"the subscriber that held the delivery must have been called")
	require.Equal(t, int32(1), sibling.Load(),
		"the in-flight config update must still reach the subscriber that unsubscribed while it was in flight")

	// That same delivery reached Gate's own subscriber (it replaced the Connect
	// runtime), so an unrelated sibling unsubscribe cannot drop a config update.
	steps.waitFor(t, "start 2", shutdownTimeout)

	// The unsubscribe did take effect: a later update does not reach the
	// sibling, while the held subscriber is called again.
	second := *cfg
	second.Connect.Name = "gate-inflight-unsubscribe-c"
	reload.FireConfigUpdate(mgr, &second, cfg)

	require.Equal(t, int32(1), sibling.Load(),
		"an unsubscribed subscriber must not be called again")
	require.Equal(t, int32(2), held.Load())
	steps.waitFor(t, "start 3", shutdownTimeout)
}
