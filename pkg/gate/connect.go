package gate

import (
	"bytes"
	"context"
	"sync"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"

	"go.minekube.com/gate/pkg/gate/config"
	"go.minekube.com/gate/pkg/internal/hashutil"
	"go.minekube.com/gate/pkg/internal/reload"
	"go.minekube.com/gate/pkg/runtime/process"
	connectcfg "go.minekube.com/gate/pkg/util/connectutil/config"
)

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
			mu          sync.Mutex
			stopConnect context.CancelFunc
			// keep track of current config hash to avoid unnecessary restarts when config didn't change
			currentConfigHash []byte
		)
		trigger := func(c *config.Config) {
			connect := c.Connect
			if connect.Enabled && !c.Editions.Java.Enabled {
				log.Info("Connect is only supported for Java edition")
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

			// stop current Connect if running
			if stopConnect != nil {
				stopConnect()
				stopConnect = nil
			}

			runnable, err := connectcfg.New(connect, instance)
			if err != nil {
				log.Error(err, "error setting up Connect")
				return
			}

			var runCtx context.Context
			runCtx, stopConnect = context.WithCancel(ctx)

			go func() {
				defer stopConnect()
				if err = runnable.Start(runCtx); err != nil {
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
		return nil
	}))
}
