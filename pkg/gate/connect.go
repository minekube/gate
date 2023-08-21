package gate

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/gate/config"
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
			name        string
		)
		trigger := func(c *config.Config) {
			cfg := c.Connect
			if cfg.Enabled && !c.Editions.Java.Enabled {
				log.Info("Connect is only supported for Java edition")
				return
			}

			mu.Lock()
			defer mu.Unlock()
			if (!cfg.Enabled && stopConnect != nil) ||
				(cfg.Enabled && (stopConnect == nil || cfg.Name != name)) {

				if stopConnect != nil {
					stopConnect()
					stopConnect = nil
				}
			}

			name = cfg.Name

			runnable, err := connectcfg.New(cfg, instance)
			if err != nil {
				log.Error(err, "error setting up Connect")
				return
			}

			var runCtx context.Context
			runCtx, stopConnect = context.WithCancel(ctx)

			go func() {
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
