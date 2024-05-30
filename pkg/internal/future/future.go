package future

import (
	"sync"
)

// Future is a struct that holds a value of type T, a slice of callbacks to be called when the value is set,
// a boolean to track if the callbacks have been called, and a mutex for thread safety.
type Future[T any] struct {
	value     T          // The value that completes the Future
	callback  []func(T)  // The callbacks that get called when the Future is completed
	completed bool       // A flag to check if the Future is completed
	mu        sync.Mutex // Mutex for thread safety
}

// New returns a new Future.
func New[T any]() *Future[T] {
	return &Future[T]{}
}

// ThenAccept registers a callback to be called when the Future is completed.
func (f *Future[T]) ThenAccept(callback func(T)) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// If the Future is already completed, call the callback
	if f.completed {
		callback(f.value)
	} else {
		// Append the new callback to the slice of callbacks
		f.callback = append(f.callback, callback)
	}
}

// Complete sets the value and calls the registered callbacks if they haven't been called yet.
func (f *Future[T]) Complete(value T) *Future[T] {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Check if the Future is already completed
	if f.completed {
		return f
	}

	// Set the value and call the callbacks
	f.value = value
	f.completed = true
	for _, fn := range f.callback {
		fn(value)
	}

	return f
}
