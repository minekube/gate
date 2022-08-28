package event

import (
	"reflect"

	"github.com/go-logr/logr"
)

// Manager is an event manager to subscribe, fire events and
// decouple the event source and sink in a complex system.
type Manager interface {
	// Subscribe subscribes a handler to an event type with a priority
	// and returns a func that can be run to unsubscribe the handler.
	//
	// HandlerFunc should return as soon as possible and start long-running tasks in parallel.
	// The Type can be any type, pointer to type or reflect.Type and the handler is only run for
	// the exact type subscribed for.
	//
	// HandlerFunc always gets the fired event of the same subscribed eventType or the same type as
	// represented by reflect.Type.
	Subscribe(eventType any, priority int, fn HandlerFunc) (unsubscribe func())
	// Fire fires an event in the calling goroutine and returns after all subscribers are complete handling it.
	// Any panic by a subscriber is caught so firing the event to the next subscriber can proceed.
	Fire(Event)
	// FireParallel fires an event in a new goroutine B and returns immediately.
	// It optionally runs handlers in goroutine B after all subscribers are done and passes
	// the potentially modified version of the fired event.
	// If an "after" handler returns false or panics no further handlers in the slice are run.
	FireParallel(event Event, after ...HandlerFunc)
	// Wait blocks until no event handlers are running.
	Wait()
	// HasSubscribers determines whether the given event has any subscribers.
	HasSubscribers(event Event) bool
}

// Subscribe subscribes a handler to an event type with a priority.
// The event type is inferred from the argument of the handler.
// See Manager.Subscribe for more details.
func Subscribe[T Event](mgr Manager, priority int, handler func(T)) (unsubscribe func()) {
	var typ T
	return mgr.Subscribe(typ, priority, func(e Event) { handler(e.(T)) })
}

// FireParallel fires an event in a new goroutine and returns immediately.
// The event type is inferred from the argument of the handler.
// See Manager.FireParallel for more details.
func FireParallel[T Event](mgr Manager, event T, after ...func(T)) {
	mgr.FireParallel(event, func(e Event) {
		ev := e.(T)
		for _, fn := range after {
			fn(ev)
		}
	})
}

// HandlerFunc is an event handler func.
type HandlerFunc func(e Event)

// Event is the event interface.
type Event any

// Type is an event type.
type Type reflect.Type

// New returns a new event Manager optionally
// using a logger to log handler panics.
func New(log logr.Logger) Manager {
	return &manager{log: log, subscribers: map[Type][]*subscriber{}}
}

// TypeOf is a helper func to get the reflect.Type from i.
// If it is nil returns nil.
func TypeOf(i any) (t Type) {
	if i == nil {
		return
	}
	switch o := i.(type) {
	case reflect.Type:
		t = o
	case reflect.Value:
		t = o.Type()
	default:
		t = reflect.TypeOf(i)
	}
	return t
}
