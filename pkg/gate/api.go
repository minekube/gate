package gate

import (
	"bytes"
	"context"
	"sync"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"

	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/gate/config"
	"go.minekube.com/gate/pkg/internal/api"
	"go.minekube.com/gate/pkg/internal/hashutil"
	"go.minekube.com/gate/pkg/internal/reload"
	"go.minekube.com/gate/pkg/runtime/process"
)

func setupAPI(cfg *config.Config, eventMgr event.Manager, initialEnable *proxy.Proxy) process.Runnable {
	return process.RunnableFunc(func(ctx context.Context) error {
		log := logr.FromContextOrDiscard(ctx).WithName("api")
		ctx = logr.NewContext(ctx, log)

		var (
			mu                sync.Mutex
			stop              context.CancelFunc
			currentConfigHash []byte
		)
		trigger := func(c *reload.ConfigUpdateEvent[config.Config]) {
			newConfigHash, err := hashutil.JsonHash(c.Config.API)
			if err != nil {
				log.Error(err, "error hashing API config")
				return
			}

			mu.Lock()
			defer mu.Unlock()

			// check if config changed
			if bytes.Equal(newConfigHash, currentConfigHash) {
				return // no change
			}
			currentConfigHash = newConfigHash

			if stop != nil {
				stop()
				stop = nil
			}

			if c.Config.API.Enabled {
				svc := api.NewService(initialEnable)
				srv := api.NewServer(c.Config.API.Config, svc)

				var runCtx context.Context
				runCtx, stop = context.WithCancel(ctx)
				go func() {
					if err := srv.Start(runCtx); err != nil {
						log.Error(err, "failed to start api service")
						return
					}
					log.Info("api service stopped")
				}()
			}
		}

		defer reload.Subscribe(eventMgr, trigger)()

		trigger(&reload.ConfigUpdateEvent[config.Config]{
			Config:     cfg,
			PrevConfig: cfg,
		})

		<-ctx.Done()
		return nil
	})
}
