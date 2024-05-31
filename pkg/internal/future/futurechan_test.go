package future

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

// RunWithTimeout runs a test function with a timeout.
func RunWithTimeout(t *testing.T, timeout time.Duration, testFunc func()) {
	done := make(chan bool)

	go func() {
		testFunc()
		done <- true
	}()

	select {
	case <-done:
		// Test completed before timeout.
	case <-time.After(timeout):
		t.Fatal("Test timed out")
	}
}

func TestNewFutureChan(t *testing.T) {
	RunWithTimeout(t, time.Second, func() {
		f := NewChan[int]()
		assert.NotNil(t, f)
		assert.NotNil(t, f.completeChan)
	})
}

func TestThenAccept(t *testing.T) {
	RunWithTimeout(t, time.Second, func() {
		result := make(chan int)
		NewChan[int]().ThenAccept(func(value int) {
			result <- value
		}).Complete(10)
		assert.Equal(t, 10, <-result)
	})
}

func TestThenApply(t *testing.T) {
	RunWithTimeout(t, time.Second, func() {
		f1 := NewChan[int]()
		f2 := ThenApply[int, int](f1, func(value int) int {
			return value * 2
		})
		f1.Complete(10)
		assert.Equal(t, 20, <-f2.Receive())
	})
}

func TestReceive(t *testing.T) {
	RunWithTimeout(t, time.Second, func() {
		f := NewChan[int]()
		go func() {
			time.Sleep(100 * time.Millisecond)
			f.Complete(10)
		}()
		assert.Equal(t, 10, <-f.Receive())
	})
}

func TestComplete(t *testing.T) {
	RunWithTimeout(t, time.Second, func() {
		f := NewChan[int]()
		f.Complete(10)
		assert.Equal(t, 10, <-f.Receive())
	})
}

func TestChan_Get(t *testing.T) {
	RunWithTimeout(t, time.Second, func() {
		f := NewChan[int]()
		go func() { time.Sleep(time.Millisecond * 100); f.Complete(10) }()
		assert.Equal(t, 10, f.Get())
	})
}
