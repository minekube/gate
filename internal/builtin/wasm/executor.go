package wasm

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"go.minekube.com/gate/internal/builtin/wasm/wasmtime"
)

var errExecutorClosed = errors.New("wasm plugin executor is closed")
var errExecutorFailed = errors.New("wasm plugin executor failed")

type executionResult struct {
	value any
	err   error
}

type executionRequest struct {
	run         func() (any, error)
	result      chan executionResult
	fatal       bool
	allowFailed bool
}

type executorFailure struct {
	err error
}

// executor is the single top-level entry lane for one component store.
// Synchronous callbacks caused by an active guest-to-host call bypass this
// queue through native.CallbackInvoker's scoped re-entry token.
type executor struct {
	mu        sync.Mutex
	runtime   componentRuntime
	requests  chan executionRequest
	done      chan struct{}
	closed    bool
	closeOnce sync.Once
	closeErr  error
	failure   atomic.Pointer[executorFailure]
	failOnce  sync.Once
	onFatal   func(error)
}

func newExecutor(runtime componentRuntime, onFatal ...func(error)) *executor {
	executor := &executor{
		runtime:  runtime,
		requests: make(chan executionRequest),
		done:     make(chan struct{}),
	}
	if len(onFatal) != 0 {
		executor.onFatal = onFatal[0]
	}
	go executor.run()
	return executor
}

func (executor *executor) run() {
	defer close(executor.done)
	for request := range executor.requests {
		if failure := executor.failureCause(); failure != nil &&
			!request.allowFailed {
			request.result <- executionResult{
				err: errors.Join(errExecutorFailed, failure),
			}
			continue
		}
		value, err := request.run()
		if request.fatal && err != nil {
			executor.fail(err)
		}
		request.result <- executionResult{value: value, err: err}
	}
}

func (executor *executor) call(run func() (any, error)) (any, error) {
	return executor.enqueue(run, false)
}

func (executor *executor) callFatal(run func() (any, error)) (any, error) {
	return executor.enqueue(run, true)
}

func (executor *executor) enqueue(
	run func() (any, error),
	fatal bool,
) (any, error) {
	request := executionRequest{
		run:    run,
		result: make(chan executionResult, 1),
		fatal:  fatal,
	}
	executor.mu.Lock()
	if executor.closed {
		executor.mu.Unlock()
		return nil, errExecutorClosed
	}
	if failure := executor.failureCause(); failure != nil {
		executor.mu.Unlock()
		return nil, errors.Join(errExecutorFailed, failure)
	}
	executor.requests <- request
	executor.mu.Unlock()
	result := <-request.result
	return result.value, result.err
}

func (executor *executor) Metadata() (native.Metadata, error) {
	value, err := executor.call(func() (any, error) {
		return executor.runtime.Metadata()
	})
	if err != nil {
		return native.Metadata{}, err
	}
	return value.(native.Metadata), nil
}

func (executor *executor) Init(contextID, proxyID uint64) error {
	_, err := executor.call(func() (any, error) {
		return nil, executor.runtime.Init(contextID, proxyID)
	})
	return err
}

func (executor *executor) SetDeadline(deadline time.Duration) error {
	_, err := executor.call(func() (any, error) {
		return nil, executor.runtime.SetDeadline(deadline)
	})
	return err
}

func (executor *executor) InvokeCallback(
	callbackTypeID uint32,
	guestID uint64,
	input []byte,
) ([]byte, error) {
	value, err := executor.callFatal(func() (any, error) {
		return executor.runtime.InvokeCallback(callbackTypeID, guestID, input)
	})
	if err != nil {
		return nil, err
	}
	return value.([]byte), nil
}

func (executor *executor) fail(failure error) {
	executor.failOnce.Do(func() {
		executor.failure.Store(&executorFailure{err: failure})
		if executor.onFatal != nil {
			executor.onFatal(failure)
		}
	})
}

func (executor *executor) failureCause() error {
	failure := executor.failure.Load()
	if failure == nil {
		return nil
	}
	return failure.err
}

func (executor *executor) Close() error {
	executor.closeOnce.Do(func() {
		executor.mu.Lock()
		executor.closed = true
		request := executionRequest{
			run: func() (any, error) {
				return nil, executor.runtime.Close()
			},
			result:      make(chan executionResult, 1),
			allowFailed: true,
		}
		executor.requests <- request
		close(executor.requests)
		executor.mu.Unlock()

		result := <-request.result
		<-executor.done
		executor.closeErr = result.err
	})
	return executor.closeErr
}
