// Package eventmgr provides the event manager Gate runs on.
//
// Gate fires its events through github.com/robinbraemer/event. In the version
// Gate depends on (v0.1.1) a Fire that overlaps an unsubscribe of a sibling
// subscriber of the same event type reads the subscriber slice while
// unsubscribe rewrites that slice in place:
//
//	manager_internal.go:139  list.subs[len(list.subs)-1] = nil  // unsubscribe
//	manager_internal.go:198  for _, sub := range list.subs      // fireSubscribers
//
// fire() releases the read lock before it delivers, so the loop can observe the
// zeroed slot and call a nil subscriber. Panic recovery is enabled by default,
// so that nil call is swallowed and the remaining subscribers of the delivery
// are silently skipped: an in-flight config update can be dropped without a log.
//
// Manager removes the class of defect instead of the instance: it keeps the
// wrapped manager at *one* subscriber per event type, a dispatcher, so the
// library's in-place mutation path is unreachable — its single-subscriber
// fast path only drops the map entry and never touches a slice that a delivery
// may be iterating. Manager keeps its own subscriber lists copy-on-write and
// snapshots them before running them, so subscribing and unsubscribing during a
// delivery is safe, and a delivery in flight is still delivered to the
// subscribers that were subscribed when it started.
//
// Delivery itself stays in the wrapped manager, so priority order, the
// any-event subscriber list, Fire/Wait/FireParallel semantics and
// HasSubscriber all keep their behavior, and panic recovery is mirrored
// (recovered per subscriber, so a panicking subscriber does not cancel the
// others of the same event type).
//
// Upstream is dormant (last release v0.1.1, 2024-11-27, no commit since) and
// not ours to patch, so the mitigation lives here; see pkg/gate/AGENTS.md.
package eventmgr

import (
	"reflect"
	"sort"
	"sync"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
)

// Manager is an event.Manager that is safe against an unsubscribe concurrent
// with a delivery of the same event type.
type Manager struct {
	mgr          event.Manager
	recoverPanic bool
	log          logr.Logger

	// mu guards types. Subscriber slices are copy-on-write: a slice handed to a
	// delivery is never modified afterwards, so deliveries read it without a lock.
	mu    sync.RWMutex
	types map[event.Type]*list
}

// list is the subscriber list of one event type.
type list struct {
	subs []*subscriber
	// unsub unsubscribes the dispatcher this list registered with the wrapped
	// manager. The wrapped manager holds exactly one subscriber per event type,
	// so this never mutates a slice a delivery might be iterating.
	unsub func()
}

type subscriber struct {
	priority int
	fn       event.HandlerFunc
}

var _ event.Manager = (*Manager)(nil)

// Option configures a Manager. Options mirror the ones of the wrapped manager.
type Option func(*options)

type options struct {
	recoverPanic bool
	log          logr.Logger
}

// WithRecoverPanic sets whether a panic of a subscriber is recovered so the
// remaining subscribers of that event type still run. Default is true, which is
// also the default of the wrapped manager.
func WithRecoverPanic(enabled bool) Option {
	return func(o *options) { o.recoverPanic = enabled }
}

// WithLogger sets the logger used to report recovered subscriber panics.
// Default is logr.Discard(), which is also the default of the wrapped manager.
func WithLogger(log logr.Logger) Option {
	return func(o *options) { o.log = log }
}

// New returns a Manager wrapping a new event manager created with the
// equivalent options.
func New(opts ...Option) *Manager {
	o := newOptions(opts...)
	return &Manager{
		mgr: event.New(
			event.WithRecoverPanic(o.recoverPanic),
			event.WithLogger(o.log),
		),
		recoverPanic: o.recoverPanic,
		log:          o.log,
		types:        make(map[event.Type]*list),
	}
}

// Safe returns an event.Manager that is safe against an unsubscribe concurrent
// with a delivery of the same event type.
//
// A nil manager yields event.Nop (no events). An already wrapped manager is
// returned unchanged, so wrapping is idempotent.
//
// The wrapped manager may hold subscribers of its own (registered directly,
// without the wrapper): those are still delivered, because the wrapper fires
// through the wrapped manager. Their order relative to the wrapper's
// subscribers is not preserved. The wrapper assumes the wrapped manager's
// default panic recovery (enabled); use New or WithRecoverPanic(false) when
// wrapping a manager built with event.WithRecoverPanic(false).
func Safe(mgr event.Manager, opts ...Option) event.Manager {
	if mgr == nil {
		return event.Nop
	}
	if m, ok := mgr.(*Manager); ok {
		return m
	}
	o := newOptions(opts...)
	return &Manager{
		mgr:          mgr,
		recoverPanic: o.recoverPanic,
		log:          o.log,
		types:        make(map[event.Type]*list),
	}
}

func newOptions(opts ...Option) options {
	o := options{recoverPanic: true, log: logr.Discard()}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// Subscribe subscribes a handler to an event type with a priority and returns a
// func that unsubscribes the handler. The returned func is safe to call more
// than once and while a delivery of the same event type is running.
func (m *Manager) Subscribe(eventType event.Event, priority int, fn event.HandlerFunc) (unsubscribe func()) {
	typ := typeOf(eventType)

	m.mu.Lock()
	defer m.mu.Unlock()

	l := m.types[typ]
	if l == nil {
		// First subscriber of this event type: register the dispatcher that
		// delivers the wrapper's subscribers for it. Registering exactly one
		// subscriber per event type is what keeps the wrapped manager's
		// in-place unsubscribe path unreachable.
		l = &list{}
		l.unsub = m.mgr.Subscribe(typ, 0, m.dispatcher(typ))
		m.types[typ] = l
	}

	sub := &subscriber{priority: priority, fn: fn}
	next := make([]*subscriber, 0, len(l.subs)+1)
	next = append(next, l.subs...)
	next = append(next, sub)
	// Same ordering as the wrapped manager: highest priority first.
	sort.Slice(next, func(i, j int) bool { return next[i].priority > next[j].priority })
	l.subs = next

	var once sync.Once
	return func() { once.Do(func() { m.unsubscribe(typ, sub) }) }
}

func (m *Manager) unsubscribe(typ event.Type, sub *subscriber) {
	m.mu.Lock()
	defer m.mu.Unlock()

	l := m.types[typ]
	if l == nil {
		return
	}
	i := -1
	for j, s := range l.subs {
		if s == sub {
			i = j
			break
		}
	}
	if i < 0 {
		return
	}

	next := make([]*subscriber, 0, len(l.subs)-1)
	next = append(next, l.subs[:i]...)
	next = append(next, l.subs[i+1:]...)
	l.subs = next
	if len(next) > 0 {
		return
	}
	delete(m.types, typ)
	l.unsub()
}

// UnsubscribeAll unsubscribes all subscribers of the given events and returns
// the number of subscribers unsubscribed. If no events are given, all
// subscribers of all events are unsubscribed. Subscribers registered directly
// with the wrapped manager are not counted.
func (m *Manager) UnsubscribeAll(events ...event.Event) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int
	if len(events) == 0 {
		for typ, l := range m.types {
			count += len(l.subs)
			delete(m.types, typ)
			l.unsub()
		}
		return count
	}

	for _, e := range events {
		typ := typeOf(e)
		l := m.types[typ]
		if l == nil {
			continue
		}
		count += len(l.subs)
		delete(m.types, typ)
		l.unsub()
	}
	return count
}

// HasSubscriber reports whether the given events have a subscriber.
func (m *Manager) HasSubscriber(events ...event.Event) bool {
	return m.mgr.HasSubscriber(events...)
}

// Fire fires an event in the calling goroutine and returns after all
// subscribers are done handling it.
func (m *Manager) Fire(event event.Event) {
	m.mgr.Fire(event)
}

// FireParallel fires an event in a new goroutine and returns immediately.
func (m *Manager) FireParallel(event event.Event, after ...event.HandlerFunc) {
	m.mgr.FireParallel(event, after...)
}

// Wait blocks until no handlers are running for the given events.
func (m *Manager) Wait(events ...event.Event) {
	m.mgr.Wait(events...)
}

// dispatcher returns the single subscriber the wrapped manager holds for typ.
// It delivers the wrapper's subscribers for that event type.
func (m *Manager) dispatcher(typ event.Type) event.HandlerFunc {
	return func(e event.Event) {
		m.mu.RLock()
		l := m.types[typ]
		var subs []*subscriber
		if l != nil {
			// Copy-on-write subscribers: the slice captured here is never
			// modified by a concurrent subscribe or unsubscribe.
			subs = l.subs
		}
		m.mu.RUnlock()

		for _, sub := range subs {
			m.call(sub, e)
		}
	}
}

// call calls a subscriber, recovering its panic if enabled. Recovery is per
// subscriber: a panicking subscriber does not cancel the remaining subscribers
// of the same event type.
func (m *Manager) call(sub *subscriber, e event.Event) {
	if m.recoverPanic {
		defer func() {
			if r := recover(); r != nil {
				m.log.Error(nil, "recovered from panic from an event subscriber",
					"panic", r,
					"eventType", typeOf(e),
					"subscriberPriority", sub.priority)
			}
		}()
	}
	sub.fn(e)
}

// typeOf returns the reflect.Type of e, the event type key of the wrapped
// manager. A nil e (the any-event subscriber list) yields a nil type.
func typeOf(e event.Event) (t event.Type) {
	switch o := e.(type) {
	case reflect.Type:
		t = o
	case reflect.Value:
		t = o.Type()
	default:
		t = reflect.TypeOf(e)
	}
	return t
}
