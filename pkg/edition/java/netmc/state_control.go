package netmc

import "sync"

// stateControl struct holds the state and a condition variable.
type stateControl struct {
	state bool       // state represents the condition to be met.
	cond  *sync.Cond // cond is a condition variable associated with a mutex.
}

// newStateControl function initializes a new stateControl with the initial state and a condition variable.
func newStateControl(initial bool) *stateControl {
	sc := &stateControl{
		state: initial,
		cond:  sync.NewCond(&sync.Mutex{}), // Initialize a new condition variable with an associated mutex.
	}
	return sc
}

// State method returns the current state.
// It locks the mutex before reading the state to ensure thread-safety.
func (s *stateControl) State() bool {
	s.cond.L.Lock()         // Lock the mutex before reading the state.
	defer s.cond.L.Unlock() // Unlock the mutex after reading the state.
	return s.state
}

// Wait method waits for the state to become true.
// It locks the mutex to ensure that the condition check and the wait operation are atomic.
func (s *stateControl) Wait() {
	s.cond.L.Lock() // Lock the mutex before checking the condition.
	for !s.state {  // If the state is not true, wait for it to become true.
		s.cond.Wait() // Wait releases the lock, suspends the goroutine, and reacquires the lock when it wakes up.
	}
	s.cond.L.Unlock() // Unlock the mutex after the condition is met.
}

// SetState method sets the state and signals all waiting goroutines.
// It locks the mutex to ensure that the state change and the signal operation are atomic.
func (s *stateControl) SetState(state bool) {
	s.cond.L.Lock()    // Lock the mutex before changing the state.
	s.state = state    // Change the state.
	s.cond.Broadcast() // Signal all waiting goroutines that the condition might be met.
	s.cond.L.Unlock()  // Unlock the mutex after changing the state and signaling the goroutines.
}
