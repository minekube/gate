package plugin

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/wasm/runtime/native"
)

type executorRuntime struct {
	mu      sync.Mutex
	entered chan uint64
	release chan struct{}
	closed  bool
	invoke  func() error
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
