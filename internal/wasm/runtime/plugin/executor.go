package plugin

import (
	"errors"
	"sync"

	"go.minekube.com/gate/internal/wasm/runtime/native"
)

var errExecutorClosed = errors.New("wasm plugin executor is closed")

type executionResult struct {
	value any
	err   error
}

type executionRequest struct {
	run    func() (any, error)
	result chan executionResult
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
}

func newExecutor(runtime componentRuntime) *executor {
	executor := &executor{
		runtime:  runtime,
		requests: make(chan executionRequest),
		done:     make(chan struct{}),
	}
	go executor.run()
	return executor
}

func (executor *executor) run() {
	defer close(executor.done)
	for request := range executor.requests {
		value, err := request.run()
		request.result <- executionResult{value: value, err: err}
	}
}

func (executor *executor) call(run func() (any, error)) (any, error) {
	request := executionRequest{
		run:    run,
		result: make(chan executionResult, 1),
	}
	executor.mu.Lock()
	if executor.closed {
		executor.mu.Unlock()
		return nil, errExecutorClosed
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

func (executor *executor) InvokeCallback(
	callbackTypeID uint32,
	guestID uint64,
	input []byte,
) ([]byte, error) {
	value, err := executor.call(func() (any, error) {
		return executor.runtime.InvokeCallback(callbackTypeID, guestID, input)
	})
	if err != nil {
		return nil, err
	}
	return value.([]byte), nil
}

func (executor *executor) Close() error {
	executor.closeOnce.Do(func() {
		executor.mu.Lock()
		executor.closed = true
		request := executionRequest{
			run: func() (any, error) {
				return nil, executor.runtime.Close()
			},
			result: make(chan executionResult, 1),
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
