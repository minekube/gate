package future

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFuture(t *testing.T) {
	t.Run("ThenAccept", func(t *testing.T) {
		f := &Future[int]{}
		var result int
		f.ThenAccept(func(value int) {
			result = value
		})
		f.Complete(10)
		assert.Equal(t, 10, result)

		// Test that the callback is immediately called when the Future is already completed
		f.ThenAccept(func(value int) {
			result = 20
		})
		assert.Equal(t, 20, result)
	})

	t.Run("Complete", func(t *testing.T) {
		f := &Future[int]{}
		f.Complete(30)
		assert.Equal(t, 30, f.value)
		assert.True(t, f.completed)
	})
}
