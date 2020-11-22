package process

import (
	"context"
	"errors"
	"fmt"
	"go.minekube.com/gate/pkg/runtime/logr"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sync"
	"time"
)

type collection struct {
	// runnables is the set of proxies that the collection injects deps into and Starts.
	runnables []Runnable

	// internalStop is the stop channel *actually* used by everything involved
	// with the collection as a stop channel, so that we can pass a stop channel
	// to things that need it off the bat (like the Channel source).
	internalStop chan struct{}

	// The logger that should be used by this collection.
	log logr.Logger

	mu      sync.Mutex // Protects these fields
	started bool
	errChan chan error

	// stop procedure engaged. In other words, we should not add anything else to the collection
	stopProcedureEngaged bool

	// gracefulShutdownTimeout is the duration given to runnable to stop
	// before the collection actually returns on stop.
	gracefulShutdownTimeout time.Duration

	// waitForRunnable is holding the number of runnables currently running so that
	// we can wait for them to exit before quitting the collection
	waitForRunnable sync.WaitGroup
}

// Add adds r to the list of Runnables to start.
// The Runnable is started if the Collection is already started.
func (pm *collection) Add(r Runnable) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.stopProcedureEngaged {
		return errors.New("can't accept new runnable as stop procedure is already engaged")
	}

	pm.runnables = append(pm.runnables, r)
	if pm.started {
		// If collection already started, start the runnable
		pm.startRunnable(r)
	}

	return nil
}

func (pm *collection) Start(stop <-chan struct{}) (err error) {
	// This chan indicates that stop is complete,
	// in other words all runnables have returned or timeout on stop request
	stopComplete := make(chan struct{})
	defer close(stopComplete)
	defer func() {
		stopErr := pm.engageStopProcedure(stopComplete)
		if stopErr != nil {
			if err != nil {
				// Aggregate allows to use errors.Is for all contained errors
				// whereas fmt.Errorf allows wrapping at most one error which means the
				// other one can not be found anymore.
				err = utilerrors.NewAggregate([]error{err, stopErr})
			} else {
				err = stopErr
			}
		}
	}()

	// Initialize this here so that we reset the signal channel state on every start.
	// Everything that might write into this channel must be started in a new goroutine,
	// because otherwise we might block this routine trying to write into the full channel
	// and will not be able to enter the deferred pm.engageStopProcedure() which drains it.
	pm.errChan = make(chan error)

	go pm.startRunnables()

	select {
	case <-stop:
		// We are done
		return nil
	case err := <-pm.errChan:
		// Error starting or running a runnable
		return err
	}
}

// engageStopProcedure signals all runnables to stop, reads potential errors
// from the errChan and waits for them to end. It must not be called more than once.
func (pm *collection) engageStopProcedure(stopComplete chan struct{}) error {
	var (
		shutdownCtx context.Context
		cancel      context.CancelFunc
	)
	if pm.gracefulShutdownTimeout > 0 {
		shutdownCtx, cancel = context.WithTimeout(context.Background(), pm.gracefulShutdownTimeout)
	} else {
		shutdownCtx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()
	close(pm.internalStop)
	// Start draining the errors before acquiring the lock to make sure we don't deadlock
	// if something that has the lock is blocked on trying to write into the unbuffered
	// channel after something else already wrote into it.
	go func() {
		for {
			select {
			case err, ok := <-pm.errChan:
				if ok {
					pm.log.Error(err, "error received after stop sequence was engaged")
				}
			case <-stopComplete:
				return
			}
		}
	}()
	if pm.gracefulShutdownTimeout == 0 {
		return nil
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.stopProcedureEngaged = true

	return pm.waitForRunnableToEnd(shutdownCtx, cancel)
}

// waitForRunnableToEnd blocks until all runnables ended or the
// gracefulShutdownTimeout was reached. In the latter case, an error is returned.
func (pm *collection) waitForRunnableToEnd(ctx context.Context, cancel context.CancelFunc) error {
	defer cancel()

	go func() {
		pm.waitForRunnable.Wait()
		cancel()
	}()

	<-ctx.Done()
	if err := ctx.Err(); err != nil && err != context.Canceled {
		return fmt.Errorf(
			"failed waiting for all runnables to end within grace period of %s: %w",
			pm.gracefulShutdownTimeout, err)
	}
	return nil
}

func (pm *collection) startRunnable(r Runnable) {
	pm.waitForRunnable.Add(1)
	go func() {
		defer pm.waitForRunnable.Done()
		if err := r.Start(pm.internalStop); err != nil {
			pm.errChan <- err
		}
	}()
}

func (pm *collection) startRunnables() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.started = true

	// Start the Runnables
	for _, c := range pm.runnables {
		// Runnables block, but we want to return an error if any have an error starting.
		// Write any Start errors to a channel so we can return them
		pm.startRunnable(c)
	}
}
