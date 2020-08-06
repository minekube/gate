package event

import (
	"go.uber.org/zap"
	"reflect"
	"sort"
	"sync"
)

// NewManager returns a new event Manager.
func NewManager() *Manager {
	return &Manager{subscribers: map[Type][]*subscriber{}}
}

// Manager is an event manager to subscribe to and fire events.
type Manager struct {
	activeSubscribers sync.WaitGroup // Wait for active subscribers to be done.

	mu sync.RWMutex // Protects following fields
	// Map subscribers sorted by priority to their event type.
	subscribers map[Type][]*subscriber
}

type subscriber struct {
	// The priority in the sorted list of other
	// subscribers handling the same event Type.
	priority int
	fn       HandlerFn // The event handler func.
}

// HandleFn is an event handler func.
type HandlerFn func(e Event)

// Type is an event type.
type Type reflect.Type

// TypeOf is a helper func to make sure the
// reflect.Type implements the Event interface
// and returns a non-pointer type.
func TypeOf(e Event) Type {
	t := reflect.TypeOf(e)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

// Event is the event interface.
type Event interface{}

// Wait blocks until no event subscribers are running.
func (m *Manager) Wait() {
	m.activeSubscribers.Wait()
}

// Subscribe subscribes a handler to an event type with a priority
// and returns a func that can be run to unsubscribe.
//
// HandlerFn should return quick and start long running tasks in parallel!
func (m *Manager) Subscribe(eventType Type, priority int, fn HandlerFn) (unsubscribe func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub := &subscriber{
		priority: priority,
		fn:       fn,
	}

	// Get subscriber list for event type
	list := append(m.subscribers[eventType], sub)
	m.subscribers[eventType] = list

	// Sort subscribers by priority
	sort.SliceStable(list, func(i, j int) bool {
		return list[j].priority < list[i].priority
	})

	// Unsubscribe func
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		list, ok := m.subscribers[eventType]
		if !ok {
			return
		}
		if len(list) == 1 {
			delete(m.subscribers, eventType)
			return
		}
		for i, s := range list {
			if s != sub { // Find by pointer
				continue
			}
			// Delete subscriber from list while maintaining the order.
			copy(list[i:], list[i+1:]) // Shift list[i+1:] left one index.
			list[len(list)-1] = nil    // Erase last element (write zero value).
			m.subscribers[eventType] = list[:len(list)-1]
			return
		}
	}
}

// FireParallel fires an event in a new goroutine and returns immediately.
// It optionally runs handlers after all subscribers are done and passes
// the potentially modified version of the fired event.
// If an after handler panics no further handlers in the slice will be run.
func (m *Manager) FireParallel(event Event, after ...HandlerFn) {
	m.activeSubscribers.Add(1)
	go func() {
		defer m.activeSubscribers.Done()
		m.Fire(event)

		defer func() {
			if r := recover(); r != nil {
				zap.S().Errorf("Recovered from panic by an after-HandlerFn for event type %s: %s",
					TypeOf(event).String(), r)
			}
		}()
		for _, fn := range after {
			fn(event)
		}
	}()
}

// Fire fires an event and returns after all subscribers are complete handling it.
// Any panic by a subscriber is caught so firing the event to the next subscriber can proceed .
func (m *Manager) Fire(event Event) {
	eventType := TypeOf(event)
	m.mu.RLock()
	list := m.subscribers[eventType]
	m.mu.RUnlock()

	for _, sub := range list {
		func() {
			defer func() {
				if r := recover(); r != nil {
					zap.L().Error("Recovered from panic from an event subscriber",
						zap.String("eventType", eventType.String()),
						zap.Int("subscriberPriority", sub.priority),
						zap.Any("panic", r))
				}
			}()
			sub.fn(event)
		}()
	}
}

// Fire fires an event in a new goroutine and
// and returns a channel immediately that receives
// the by subscribers modified version of the fired event.
/*func (m *Manager) FireParallel(event Event) (resultChan <-chan Event) {
	result := make(chan Event, 1)
	eventType := reflect.TypeOf(event)

	m.mu.RLock()
	defer m.mu.RUnlock()
	list, ok := m.subscribers[eventType]
	if !ok || len(list) == 0 { // Don't have to start a goroutine if there is no subscriber
		result <- event // No modification, return as is
		return result
	}

	m.activeSubscribers.Add(1)
	go func() {
		defer m.activeSubscribers.Done()
		m.mu.RLock()
		list := m.subscribers[eventType]
		m.mu.RUnlock()
		for _, sub := range list {
			sub.handler(event)
		}
		result <- event // Return potentially modified version
	}()
	return result
}
*/
