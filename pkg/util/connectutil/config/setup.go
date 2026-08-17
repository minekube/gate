package config

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/go-logr/logr"

	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/process"
)

// Retry policy for the Connect watch client.
const (
	// retryAfterBase is the initial retry delay after a failure.
	retryAfterBase = 5 * time.Second
	// retryAfterMax caps the exponential backoff.
	retryAfterMax = 2 * time.Minute
	// maxConsecutiveAuthRejections is the number of consecutive HTTP 401
	// rejections after which the watch client stops retrying for an
	// endpoint. A 401 means the server permanently rejects the
	// (endpoint, token) pair — retrying at any short interval only
	// produces a 401 storm (displaced connectors) without any chance of
	// success until the operator fixes the endpoint/token.
	maxConsecutiveAuthRejections = 3
)

// healthySessionDuration is the minimum uptime of a watch session after
// which its end resets the retry backoff (the session was healthy; the
// disconnect is treated as a fresh event). It is a variable so tests can
// shorten it.
var healthySessionDuration = time.Minute

// authRejectedError marks a watch failure caused by the server permanently
// rejecting this endpoint's credentials (HTTP 401). retryingRunnable treats
// it as terminal for the endpoint after maxConsecutiveAuthRejections
// consecutive rejections and stops retrying.
type authRejectedError struct {
	endpoint string
	err      error
}

func (e *authRejectedError) Error() string { return e.err.Error() }
func (e *authRejectedError) Unwrap() error { return e.err }

func retryingRunnable(r process.Runnable, afterFns ...func()) process.Runnable {
	return process.RunnableFunc(func(ctx context.Context) error {
		log := logr.FromContextOrDiscard(ctx).WithName("retry")
		defer func() {
			for _, fn := range afterFns {
				fn()
			}
		}()

		var err error
		consecutiveFailures := 0
		consecutiveAuthRejections := 0
		for {
			start := time.Now()
			if err = r.Start(ctx); err != nil {
				select {
				case <-ctx.Done():
					return err
				default:
				}

				// A session that stayed up for a while was healthy: its end
				// (e.g. server restart) restarts the backoff so reconnect is
				// fast again, instead of inheriting a grown delay.
				if time.Since(start) > healthySessionDuration {
					consecutiveFailures = 0
					consecutiveAuthRejections = 0
				}

				consecutiveFailures++
				var authErr *authRejectedError
				if errors.As(err, &authErr) {
					consecutiveAuthRejections++
					if consecutiveAuthRejections >= maxConsecutiveAuthRejections {
						// Terminal-ish: the server permanently rejects this
						// (endpoint, token). Stop retrying so we stop
						// hammering the server. The error propagates up and
						// only stops the Connect watch subsystem (Gate keeps
						// running); a config reload or restart re-creates
						// the client once the operator fixed the endpoint.
						log.Error(err, "Watch endpoint permanently rejected (401); stopping retries until endpoint/token is fixed",
							"endpoint", authErr.endpoint)
						return err
					}
				} else {
					consecutiveAuthRejections = 0
				}

				after := jitter(backoffDelay(consecutiveFailures))
				// Backoff reconnect without logging an error after 5 times.
				if consecutiveFailures <= 5 {
					log.Info("Error while running process, retrying...",
						"error", err, "retryAfter", after.String())
				} else {
					log.V(1).Info("Error while running process, retrying...",
						"error", err, "retryAfter", after.String())
				}
				sleep(ctx, after)
				continue // retry
			}
			return nil
		}
	})
}

// backoffDelay returns the retry delay for the n-th consecutive failure:
// 5s, 10s, 20s, ... capped at retryAfterMax.
func backoffDelay(n int) time.Duration {
	d := retryAfterBase
	for i := 1; i < n; i++ {
		d *= 2
		if d >= retryAfterMax {
			return retryAfterMax
		}
	}
	return d
}

// jitter adds ±20% random jitter to a delay to desynchronize concurrently
// retrying clients (e.g. many displaced connectors reconnecting at once).
// It is a variable so tests can replace it with the identity function.
var jitter = func(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	delta := d / 5
	return d - delta + time.Duration(rand.Int63n(int64(2*delta)+1))
}

// sleep waits for d or until ctx is done. It is a variable so tests can
// replace it with a recorder to observe retry delays without waiting.
var sleep = func(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

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
