package wasm

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/builtin/wasm/wasmtime"
)

type executorRuntime struct {
	mu      sync.Mutex
	entered chan uint64
	release chan struct{}
	closed  bool
	invoke  func() error
}

type queuedFailureRuntime struct {
	executorRuntime
	failure error
	release chan struct{}
}

func (runtime *queuedFailureRuntime) InvokeCallback(
	_ uint32,
	guestID uint64,
	input []byte,
) ([]byte, error) {
	runtime.entered <- guestID
	if guestID == 1 {
		<-runtime.release
		return nil, runtime.failure
	}
	return append([]byte(nil), input...), nil
}

func (runtime *executorRuntime) Metadata() (native.Metadata, error) {
	return native.Metadata{Name: "executor-test"}, nil
}

func (runtime *executorRuntime) Init(uint64, uint64) error {
	return nil
}

func (runtime *executorRuntime) SetDeadline(time.Duration) error {
	return nil
}

func (runtime *executorRuntime) InvokeCallback(
	_ uint32,
	guestID uint64,
	input []byte,
) ([]byte, error) {
	runtime.entered <- guestID
	if runtime.invoke != nil {
		if err := runtime.invoke(); err != nil {
			return nil, err
		}
	}
	if runtime.release != nil {
		<-runtime.release
	}
	return append([]byte(nil), input...), nil
}

func (runtime *executorRuntime) Close() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.closed = true
	return nil
}

func TestExecutorRunsTopLevelCallbacksInQueueOrder(t *testing.T) {
	t.Parallel()

	runtime := &executorRuntime{
		entered: make(chan uint64, 3),
		release: make(chan struct{}, 3),
	}
	executor := newExecutor(runtime)
	t.Cleanup(func() { require.NoError(t, executor.Close()) })

	results := make(chan uint64, 3)
	for id := uint64(1); id <= 3; id++ {
		id := id
		go func() {
			_, err := executor.InvokeCallback(1, id, []byte{byte(id)})
			require.NoError(t, err)
			results <- id
		}()
		require.Equal(t, id, <-runtime.entered)
		if id < 3 {
			runtime.release <- struct{}{}
			require.Equal(t, id, <-results)
		}
	}
	runtime.release <- struct{}{}
	require.Equal(t, uint64(3), <-results)
}

func TestExecutorCloseRejectsNewCallsAndClosesRuntime(t *testing.T) {
	t.Parallel()

	runtime := &executorRuntime{entered: make(chan uint64, 1)}
	executor := newExecutor(runtime)
	require.NoError(t, executor.Close())
	require.NoError(t, executor.Close())

	_, err := executor.InvokeCallback(1, 1, nil)
	require.ErrorIs(t, err, errExecutorClosed)
	runtime.mu.Lock()
	require.True(t, runtime.closed)
	runtime.mu.Unlock()
}

func TestExecutorReturnsRuntimeErrors(t *testing.T) {
	t.Parallel()

	expected := errors.New("init failed")
	runtime := &fakeRuntime{
		init: func() error { return expected },
	}
	executor := newExecutor(runtime)
	t.Cleanup(func() { require.NoError(t, executor.Close()) })

	require.ErrorIs(t, executor.Init(1, 2), expected)
}

func TestExecutorMarksRuntimeCallbackFailureFatal(t *testing.T) {
	t.Parallel()

	expected := errors.New("component trapped")
	fatal := make(chan error, 1)
	runtime := &executorRuntime{
		entered: make(chan uint64, 1),
		invoke:  func() error { return expected },
	}
	executor := newExecutor(runtime, func(err error) { fatal <- err })
	t.Cleanup(func() { require.NoError(t, executor.Close()) })

	_, err := executor.InvokeCallback(1, 1, nil)
	require.ErrorIs(t, err, expected)
	require.ErrorIs(t, <-fatal, expected)

	_, err = executor.InvokeCallback(1, 2, nil)
	require.ErrorIs(t, err, errExecutorFailed)
}

func TestExecutorRejectsQueuedCallbackAfterFatalFailure(t *testing.T) {
	t.Parallel()

	expected := errors.New("component trapped")
	runtime := &queuedFailureRuntime{
		executorRuntime: executorRuntime{entered: make(chan uint64, 2)},
		failure:         expected,
		release:         make(chan struct{}),
	}
	fatal := make(chan error, 1)
	executor := newExecutor(runtime, func(err error) { fatal <- err })
	t.Cleanup(func() { require.NoError(t, executor.Close()) })

	first := make(chan error, 1)
	go func() {
		_, err := executor.InvokeCallback(1, 1, nil)
		first <- err
	}()
	require.Equal(t, uint64(1), <-runtime.entered)

	second := make(chan error, 1)
	go func() {
		_, err := executor.InvokeCallback(1, 2, nil)
		second <- err
	}()
	require.Eventually(t, func() bool {
		if executor.mu.TryLock() {
			executor.mu.Unlock()
			return false
		}
		return true
	}, time.Second, time.Millisecond, "second callback must be queued")

	close(runtime.release)
	require.ErrorIs(t, <-first, expected)
	require.ErrorIs(t, <-fatal, expected)
	require.ErrorIs(t, <-second, errExecutorFailed)
	select {
	case id := <-runtime.entered:
		t.Fatalf("queued callback %d entered the failed runtime", id)
	default:
	}
}
