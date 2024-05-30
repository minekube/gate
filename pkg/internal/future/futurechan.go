package future

import "sync"

// Chan represents a future that will complete with a single value of type T.
// This struct is non-blocking and allows for parallel callbacks to be registered.
// The result of the future can be accessed via the Get() method or by receiving
// from the channel returned by the Receive() method.
// The future can be completed with a value of type T using the Complete() method.
// The future can be chained with other futures using the ThenAccept() and ThenApply() methods.
// The future is thread-safe and can be used concurrently.
// The ThenAccept(), Get() and Receive() methods can be called multiple times to receive the same result.
// Multiple calls to the Complete() method will only complete the future once.
type Chan[T any] struct {
	result func() T // gets the result of the future

	complete     sync.Once // guards the completeChan
	completeChan chan T    // channel to complete the future
}

// NewChan returns a new Chan that will complete with a single value of type T.
func NewChan[T any]() *Chan[T] {
	completeChan := make(chan T, 1)
	return &Chan[T]{
		completeChan: completeChan,
		result:       sync.OnceValue[T](func() T { return <-completeChan }),
	}
}

// ThenAccept registers a parallel callback to be called when the future is completed.
// This method is non-blocking and returns immediately.
func (f *Chan[T]) ThenAccept(callback func(T)) *Chan[T] {
	go func() { callback(f.result()) }()
	return f
}

// ThenApply returns a new future that will complete with the
// result of the callback applied to the result of this future.
// This method is non-blocking and returns immediately.
func ThenApply[O1, O2 any](f *Chan[O1], callback func(O1) O2) *Chan[O2] {
	f2 := NewChan[O2]()
	f.ThenAccept(func(o1 O1) { f2.Complete(callback(o1)) })
	return f2
}

// Receive returns a channel that will receive the result of this future.
// The channel will be closed once after the result was sent.
// Multiple calls to this method will return new channels that will receive the same result.
func (f *Chan[T]) Receive() <-chan T {
	c := make(chan T, 1)
	go func() { c <- f.result(); close(c) }()
	return c
}

// Get returns the result of this future or blocks indefinitely until the result is available.
// Multiple calls to this method will all block until the result is available.
func (f *Chan[T]) Get() T { return f.result() }

// Complete sets the result of this future once and propagates it to all current and future listeners.
// Multiple calls to this method will not have any effect.
// This method is non-blocking and returns immediately.
func (f *Chan[T]) Complete(result T) *Chan[T] {
	f.complete.Do(func() { f.completeChan <- result; close(f.completeChan) })
	return f
}

// Completed returns a new completed future with the given result.
func Completed[T any](result T) *Chan[T] {
	return NewChan[T]().Complete(result)
}
