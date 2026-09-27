package gate

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"

	"go.minekube.com/gate/pkg/gate/config"
	"go.minekube.com/gate/pkg/internal/hashutil"
	"go.minekube.com/gate/pkg/internal/reload"
	"go.minekube.com/gate/pkg/runtime/process"
	connectcfg "go.minekube.com/gate/pkg/util/connectutil/config"
)

// connectStopTimeout bounds how long a Connect runtime is given to stop before
// Gate stops waiting for it. It stays well below the process collection's own
// graceful shutdown period (30s) so a Connect runtime that cannot stop does not
// push Gate's own shutdown past its grace period.
const connectStopTimeout = 10 * time.Second

// errConnectStopTimeout reports a Connect runtime that did not stop in time.
var errConnectStopTimeout = errors.New("Connect did not stop within the shutdown timeout")

// newConnectRuntime builds the Connect runtime for a config. It is a variable so
// tests can substitute a runtime whose shutdown they control.
var newConnectRuntime = connectcfg.New

// connectRuntime is a running Connect runtime (watch client, optionally the
// self-hosted Connect service) whose shutdown can be awaited.
type connectRuntime struct {
	stop context.CancelFunc
	// done is closed once the runtime's runnable returned.
	done chan struct{}
}

// Setup Connect with reload support
func setupConnect(
	coll process.Collection,
	c *config.Config,
	eventMgr event.Manager,
	instance connectcfg.Instance,
) error {
	return coll.Add(process.RunnableFunc(func(ctx context.Context) error {
		log := logr.FromContextOrDiscard(ctx).WithName("connect")
		ctx = logr.NewContext(ctx, log)

		var (
			mu sync.Mutex
			// running is the runtime started for the current Connect config,
			// nil if none is running.
			running *connectRuntime
			// keep track of current config hash to avoid unnecessary restarts when config didn't change
			currentConfigHash []byte
		)

		// stopRunning stops the current runtime and waits (bounded) for it to
		// return, so a stopped or superseded connector can never keep running:
		// it would keep dialing the watch service and rewriting the token file
		// after Gate reported that it stopped. Callers hold mu.
		stopRunning := func() {
			if running == nil {
				return
			}
			r := running
			running = nil
			r.stop()
			select {
			case <-r.done:
			case <-time.After(connectStopTimeout):
				log.Error(errConnectStopTimeout,
					"Connect is still running after the stop timeout",
					"timeout", connectStopTimeout.String())
			}
		}

		trigger := func(c *config.Config) {
			connect := c.Connect
			// Connect is always supported now that Java is embedded
			if !connect.Enabled {
				return
			}

			newConfigHash, err := hashutil.JsonHash(connect)
			if err != nil {
				log.Error(err, "error hashing Connect config")
				return
			}

			mu.Lock()
			defer mu.Unlock()

			// check if config changed
			if bytes.Equal(newConfigHash, currentConfigHash) {
				return // no change
			}
			currentConfigHash = newConfigHash

			// Stop the current Connect and wait for it before starting its
			// replacement: two connectors must never watch for the same
			// endpoint (and provision the same token file) at once.
			stopRunning()

			runnable, err := newConnectRuntime(connect, instance)
			if err != nil {
				log.Error(err, "error setting up Connect")
				return
			}

			runCtx, stop := context.WithCancel(ctx)
			r := &connectRuntime{stop: stop, done: make(chan struct{})}
			running = r

			go func() {
				defer close(r.done)
				defer stop()
				if err := runnable.Start(runCtx); err != nil {
					log.Error(err, "error with Connect")
					return
				}
				log.Info("connect stopped")
			}()
		}

		defer reload.Subscribe(eventMgr, func(c *reload.ConfigUpdateEvent[config.Config]) {
			trigger(c.Config)
		})()

		trigger(c)

		<-ctx.Done()
		mu.Lock()
		defer mu.Unlock()
		// Gate is stopped only after the Connect runtime it started returned.
		stopRunning()
		return nil
	}))
}
