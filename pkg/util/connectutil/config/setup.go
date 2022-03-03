package config

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/logr"
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
func New(c Config, log logr.Logger, inst Instance) (process.Runnable, error) {
	coll := process.New(process.Options{AllOrNothing: true})

	if c.Enabled {
		client, err := connectClient(c, log.WithName("watch"), inst)
		if err != nil {
			return nil, fmt.Errorf("could not prepare Connect client: %w", err)
		}
		_ = coll.Add(client)
	}
	if c.Service.Enabled {
		svc, err := service(c, log.WithName("service"), inst)
		if err != nil {
			return nil, fmt.Errorf("could not prepare Connect service: %w", err)
		}
		_ = coll.Add(svc)
	}

	return coll, nil
}

func retryingRunnable(log logr.Logger, r process.Runnable, afterFns ...func()) process.Runnable {
	log = log.WithName("retry")
	const after = time.Second * 5
	return process.RunnableFunc(func(stop <-chan struct{}) error {
		defer func() {
			for _, fn := range afterFns {
				fn()
			}
		}()

		var err error
		for {
			if err = r.Start(stop); err != nil {
				select {
				case <-stop:
					return err
				default:
					log.Info("Error while running process, retrying...",
						"error", err, "retryAfter", after.String())
					sleep(stop, after)
					continue // retry
				}
			}
			return nil
		}
	})
}

func ctxRunnable(r func(ctx context.Context) error) process.Runnable {
	return process.RunnableFunc(func(stop <-chan struct{}) error {
		ctx, cancel := ctxFromChan(stop)
		defer cancel()
		return r(ctx)
	})
}

func ctxFromChan(stop <-chan struct{}) (ctx context.Context, cancel context.CancelFunc) {
	ctx, cancel = context.WithCancel(context.Background())
	go func() {
		defer cancel()
		select {
		case <-stop:
		case <-ctx.Done():
		}
	}()
	return
}

func sleep(stop <-chan struct{}, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-stop:
	}
}
