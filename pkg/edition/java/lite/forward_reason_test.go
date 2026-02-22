package lite

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return false }

func TestClassifyForwardEnd(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		reason := classifyForwardEnd(context.Background(), copyResult{
			dir: pipeClientToBackend,
			err: timeoutErr{},
		})
		assert.Equal(t, Timeout, reason)
	})

	t.Run("shutdown", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		reason := classifyForwardEnd(ctx, copyResult{
			dir: pipeClientToBackend,
			err: errors.New("context cancelled"),
		})
		assert.Equal(t, Shutdown, reason)
	})

	t.Run("backend closed", func(t *testing.T) {
		reason := classifyForwardEnd(context.Background(), copyResult{
			dir: pipeBackendToClient,
			err: nil,
		})
		assert.Equal(t, BackendClosed, reason)
	})

	t.Run("client closed", func(t *testing.T) {
		reason := classifyForwardEnd(context.Background(), copyResult{
			dir: pipeClientToBackend,
			err: nil,
		})
		assert.Equal(t, ClientClosed, reason)
	})

	t.Run("error", func(t *testing.T) {
		reason := classifyForwardEnd(context.Background(), copyResult{
			dir: pipeClientToBackend,
			err: errors.New("boom"),
		})
		assert.Equal(t, Error, reason)
	})
}
