package config

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/go-logr/logr"

	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/process"
)

type Instance interface {
	proxy.ServerRegistry
	ConnHandler
}

type ConnHandler interface {
	HandleConn(conn net.Conn)
}

// New validates the config and creates a process collection from it.
func New(c Config, inst Instance) (process.Runnable, error) {
	coll := process.New(process.Options{AllOrNothing: true})

	if c.Enabled {
		client, err := connectClient(c, inst)
		if err != nil {
			return nil, fmt.Errorf("could not prepare Connect client: %w", err)
		}
		_ = coll.Add(process.RunnableFunc(func(ctx context.Context) error {
			ctx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithName("watch"))
			return client.Start(ctx)
		}))
	}
	if c.Service.Enabled {
		svc, err := service(c, inst)
		if err != nil {
			return nil, fmt.Errorf("could not prepare Connect service: %w", err)
		}
		_ = coll.Add(process.RunnableFunc(func(ctx context.Context) error {
			ctx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithName("service"))
			return svc.Start(ctx)
		}))
	}

	return coll, nil
}

func retryingRunnable(r process.Runnable, afterFns ...func()) process.Runnable {
	return process.RunnableFunc(func(ctx context.Context) error {
		log := logr.FromContextOrDiscard(ctx).WithName("retry")
		const after = time.Second * 5
		defer func() {
			for _, fn := range afterFns {
				fn()
			}
		}()

		var err error
		for {
			if err = r.Start(ctx); err != nil {
				select {
				case <-ctx.Done():
					return err
				default:
					log.Info("Error while running process, retrying...",
						"error", err, "retryAfter", after.String())
					sleep(ctx, after)
					continue // retry
				}
			}
			return nil
		}
	})
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}
