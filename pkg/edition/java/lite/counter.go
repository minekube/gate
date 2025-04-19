package lite

import (
	"sync/atomic"
)

// ConnectionCounter tracks the number of active connections in LiteMode
var ConnectionCounter = &counter{}

// counter is a thread-safe counter for connections
type counter struct {
	count int32
}

// Increment increments the counter by 1 and returns the new value
func (c *counter) Increment() int32 {
	return atomic.AddInt32(&c.count, 1)
}

// Decrement decrements the counter by 1 and returns the new value
func (c *counter) Decrement() int32 {
	return atomic.AddInt32(&c.count, -1)
}

// Count returns the current count
func (c *counter) Count() int32 {
	return atomic.LoadInt32(&c.count)
} 