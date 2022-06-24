package event

import (
	"sort"
	"sync"

	"github.com/go-logr/logr"
)

type manager struct {
	activeSubscribers sync.WaitGroup // Wait for active subscribers to be done.
	log               logr.Logger    // may be nil

	mu sync.RWMutex // Protects following fields
	// Map subscribers sorted by priority to their event type.
	subscribers map[Type][]*subscriber
}

type subscriber struct {
	// The priority in the sorted list of other
	// subscribers handling the same event Type.
	priority int
	fn       HandlerFunc // The event handler func.
}

func (m *manager) Wait() {
	m.activeSubscribers.Wait()
}

func (m *manager) HasSubscribers(event Event) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.subscribers[TypeOf(event)]) != 0
}

func (m *manager) Subscribe(eventType any, priority int, fn HandlerFunc) (unsubscribe func()) {
	eType := TypeOf(eventType)
	if eType == nil {
		return func() {}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sub := &subscriber{
		priority: priority,
		fn:       fn,
	}

	// Get-add subscriber list for event type
	list := append(m.subscribers[eType], sub)
	m.subscribers[eType] = list

	// Sort subscribers by priority
	sort.SliceStable(list, func(i, j int) bool {
		return list[j].priority < list[i].priority
	})

	// Unsubscribe func
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			list, ok := m.subscribers[eType]
			if !ok {
				return
			}
			if len(list) == 1 {
				delete(m.subscribers, eType)
				return
			}
			for i, s := range list {
				if s != sub { // Find by pointer
					continue
				}
				// Delete subscriber from list while maintaining the order.
				copy(list[i:], list[i+1:]) // Shift list[i+1:] left one index.
				list[len(list)-1] = nil    // Erase last element (write zero value).
				m.subscribers[eType] = list[:len(list)-1]
				return
			}
		})
	}
}

func (m *manager) FireParallel(event Event, after ...HandlerFunc) {
	m.activeSubscribers.Add(1)
	go func() {
		defer m.activeSubscribers.Done()
		m.Fire(event)

		var i int
		defer func() {
			if r := recover(); r != nil {
				m.log.Error(nil,
					"Recovered from panic by an 'after fire' func",
					"panic", r,
					"eventType", TypeOf(event),
					"index", i)
			}
		}()
		var fn HandlerFunc
		for i, fn = range after {
			fn(event)
		}
	}()
}

func (m *manager) Fire(event Event) {
	eventType := TypeOf(event)
	m.mu.RLock()
	list := m.subscribers[eventType]
	m.mu.RUnlock()

	for _, sub := range list {
		func() {
			defer func() {
				if r := recover(); r != nil {
					m.log.Error(nil, "Recovered from panic from an event subscriber",
						"panic", r,
						"eventType", eventType,
						"subscriberPriority", sub.priority)
				}
			}()
			sub.fn(event)
		}()
	}
}

// Fire fires an event in a new goroutine and
// and returns a channel immediately that receives
// the by subscribers modified version of the fired event.
/*func (m *manager) FireParallel(event Event) (resultChan <-chan Event) {
	result := make(chan Event, 1)
	Type := reflect.TypeOf(event)

	m.mu.RLock()
	defer m.mu.RUnlock()
	list, ok := m.subscribers[Type]
	if !ok || len(list) == 0 { // Don't have to start a goroutine if there is no subscriber
		result <- event // No modification, return as is
		return result
	}

	m.activeSubscribers.Add(1)
	go func() {
		defer m.activeSubscribers.Done()
		m.mu.RLock()
		list := m.subscribers[Type]
		m.mu.RUnlock()
		for _, sub := range list {
			sub.handler(event)
		}
		result <- event // Return potentially modified version
	}()
	return result
}
*/
