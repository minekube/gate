package eventmgr

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"
)

// These tests pin the contract of Gate's event manager (pkg/util/eventmgr):
// a Fire that is concurrent with the unsubscribe of a *sibling* subscriber of
// the same event type must not lose that delivery, skip subscribers, or call a
// zeroed subscriber slot. TestUpstreamEventManagerLosesSiblingDeliveryInFlight
// shows the defect of the wrapped manager (robinbraemer/event v0.1.1) that this
// package exists for; TestInFlightDeliveryReachesSubscribersPresentAtFireStart
// is the same interleaving against Manager.

// inFlightProbeTimeout bounds the forced interleavings. It is orders of
// magnitude above the work they need; elapsing means the delivery is stuck.
const inFlightProbeTimeout = 10 * time.Second

// siblingEvent is the event type the tests deliver.
type siblingEvent struct{}

// deliveryOutcome is what one forced in-flight interleaving observed.
type deliveryOutcome struct {
	// panicked is what the firing goroutine recovered, nil if it did not panic.
	panicked any
	// held is how often the subscriber that held the delivery was called.
	held int32
	// sibling is how often the subscriber that unsubscribed mid-delivery was
	// called by that delivery.
	sibling int32
}

// forceSiblingUnsubscribeInFlight fires one siblingEvent and holds the delivery
// inside its first subscriber while its second subscriber — a sibling of the
// same event type — unsubscribes. The interleaving is forced, not raced: the
// sibling only unsubscribes after the held subscriber reported that the
// delivery started, and the held subscriber only returns afterwards, so the
// unsubscribe provably lands while the delivery is in flight.
func forceSiblingUnsubscribeInFlight(t *testing.T, mgr event.Manager) deliveryOutcome {
	t.Helper()

	var (
		outcome    deliveryOutcome
		delivering = make(chan struct{})
		release    = make(chan struct{})
		held       atomic.Int32
		sibling    atomic.Int32
	)

	// Priority 1, so the held subscriber is called before the sibling.
	event.Subscribe(mgr, 1, func(*siblingEvent) {
		held.Add(1)
		close(delivering)
		<-release
	})
	unsubscribeSibling := event.Subscribe(mgr, 0, func(*siblingEvent) { sibling.Add(1) })

	fired := make(chan struct{})
	go func() {
		defer close(fired)
		defer func() { outcome.panicked = recover() }()
		mgr.Fire(&siblingEvent{})
	}()

	select {
	case <-delivering:
	case <-time.After(inFlightProbeTimeout):
		t.Fatal("the event was never delivered to its first subscriber")
	}

	// The delivery is in flight right now: this unsubscribe overlaps it.
	unsubscribeSibling()
	close(release)

	select {
	case <-fired:
	case <-time.After(inFlightProbeTimeout):
		t.Fatal("the in-flight delivery never returned")
	}

	outcome.held = held.Load()
	outcome.sibling = sibling.Load()
	return outcome
}

// TestUpstreamEventManagerLosesSiblingDeliveryInFlight pins the defect of the
// wrapped manager (robinbraemer/event v0.1.1) that Manager mitigates: its
// unsubscribe rewrites the subscriber slice of a multi-subscriber list in
// place while a delivery is iterating it, so the delivery reaches the zeroed
// slot, calls a nil subscriber, and panics — with panic recovery enabled (the
// default) that panic is swallowed and the remaining subscribers of the
// delivery are silently skipped instead.
//
// THIS TEST ASSERTS UPSTREAM BEHAVIOR. When it starts failing, upstream fixed
// the race (or the module was bumped to a fixed version): drop the mitigation,
// verify with `go test -race ./pkg/gate/ ./pkg/util/eventmgr/...`, and use the
// upstream manager directly.
func TestUpstreamEventManagerLosesSiblingDeliveryInFlight(t *testing.T) {
	// Recovery disabled, so the nil subscriber the delivery reads panics here
	// instead of being swallowed.
	upstream := event.New(event.WithRecoverPanic(false))

	outcome := forceSiblingUnsubscribeInFlight(t, upstream)

	require.NotNil(t, outcome.panicked,
		"the in-flight delivery should have called the zeroed subscriber slot")
	require.Equal(t, int32(1), outcome.held,
		"the subscriber that held the delivery must have been called")
	require.Zero(t, outcome.sibling,
		"the unsubscribed sibling must have been skipped by the panicking delivery")
}

// TestInFlightDeliveryReachesSubscribersPresentAtFireStart is the contract of
// Manager: the same forced interleaving must deliver the event to every
// subscriber that was subscribed when the delivery started — including the
// sibling that unsubscribed while it was in flight — and must not panic.
func TestInFlightDeliveryReachesSubscribersPresentAtFireStart(t *testing.T) {
	requireOutcome := func(t *testing.T, outcome deliveryOutcome) {
		t.Helper()
		require.Nil(t, outcome.panicked,
			"an in-flight delivery must not panic when a sibling subscriber unsubscribes")
		require.Equal(t, int32(1), outcome.held,
			"the subscriber that held the delivery must have been called")
		require.Equal(t, int32(1), outcome.sibling,
			"the delivery must still reach the sibling that unsubscribed while it was in flight")
	}

	t.Run("Manager", func(t *testing.T) {
		requireOutcome(t, forceSiblingUnsubscribeInFlight(t, New()))
	})

	t.Run("Safe over an upstream manager", func(t *testing.T) {
		// What pkg/gate does with a caller-provided manager.
		requireOutcome(t, forceSiblingUnsubscribeInFlight(t, Safe(event.New())))
	})
}

// TestUnsubscribedSiblingIsNotDeliveredAgain pins that the sibling really is
// unsubscribed: only the delivery that was already in flight reaches it.
func TestUnsubscribedSiblingIsNotDeliveredAgain(t *testing.T) {
	mgr := New()

	var sibling, other atomic.Int32
	unsubscribeSibling := event.Subscribe(mgr, 0, func(*siblingEvent) { sibling.Add(1) })
	event.Subscribe(mgr, 0, func(*siblingEvent) { other.Add(1) })

	mgr.Fire(&siblingEvent{})
	require.Equal(t, int32(1), sibling.Load())

	unsubscribeSibling()
	mgr.Fire(&siblingEvent{})

	require.Equal(t, int32(1), sibling.Load(), "an unsubscribed subscriber must not be called again")
	require.Equal(t, int32(2), other.Load(), "the other subscriber must still be called")
}

// TestSubscribersAreCalledInPriorityOrder pins the delivery order of the
// wrapped manager: highest priority first, registration order among equals.
func TestSubscribersAreCalledInPriorityOrder(t *testing.T) {
	mgr := New()

	var (
		mu     sync.Mutex
		called []string
	)
	record := func(name string) func(*siblingEvent) {
		return func(*siblingEvent) {
			mu.Lock()
			defer mu.Unlock()
			called = append(called, name)
		}
	}

	event.Subscribe(mgr, 1, record("low"))
	event.Subscribe(mgr, 5, record("high"))
	event.Subscribe(mgr, 3, record("mid"))
	event.Subscribe(mgr, 3, record("mid-second"))

	mgr.Fire(&siblingEvent{})

	require.Equal(t, []string{"high", "mid", "mid-second", "low"}, called)
}

// TestAnyEventSubscribersAreCalledFirst pins that the any-event subscriber
// list keeps its precedence over the typed list, regardless of priority.
func TestAnyEventSubscribersAreCalledFirst(t *testing.T) {
	mgr := New()

	var (
		mu     sync.Mutex
		called []string
	)
	record := func(name string) event.HandlerFunc {
		return func(event.Event) {
			mu.Lock()
			defer mu.Unlock()
			called = append(called, name)
		}
	}

	// A typed subscriber, not the helper above: an untyped handler func is an
	// any-event subscriber, which is the list under test.
	event.Subscribe(mgr, 100, func(*siblingEvent) {
		mu.Lock()
		defer mu.Unlock()
		called = append(called, "typed")
	})
	mgr.Subscribe(nil, 0, record("any"))

	mgr.Fire(&siblingEvent{})

	require.Equal(t, []string{"any", "typed"}, called)
}

// TestHasSubscriberSeesWrappedSubscribers pins HasSubscriber, which the proxy
// uses to skip building events nobody subscribed to.
func TestHasSubscriberSeesWrappedSubscribers(t *testing.T) {
	mgr := New()
	require.False(t, mgr.HasSubscriber())
	require.False(t, mgr.HasSubscriber(&siblingEvent{}))

	unsubscribeTyped := event.Subscribe(mgr, 0, func(*siblingEvent) {})
	require.True(t, mgr.HasSubscriber(&siblingEvent{}))
	require.False(t, mgr.HasSubscriber(&otherEvent{}),
		"a different event type has no subscriber")
	require.True(t, mgr.HasSubscriber())

	unsubscribeTyped()
	require.False(t, mgr.HasSubscriber(&siblingEvent{}),
		"a fully unsubscribed type must not report a subscriber")
	require.False(t, mgr.HasSubscriber())

	// An any-event subscriber is subscribed to every event type.
	unsubscribeAny := mgr.Subscribe(nil, 0, func(event.Event) {})
	require.True(t, mgr.HasSubscriber(&otherEvent{}))

	unsubscribeAny()
	require.False(t, mgr.HasSubscriber(&otherEvent{}))
	require.False(t, mgr.HasSubscriber())
}

// TestUnsubscribeAllCountsAndStopsDelivery pins UnsubscribeAll's count and that
// it drops the subscribers it removed, including the any-event list.
func TestUnsubscribeAllCountsAndStopsDelivery(t *testing.T) {
	mgr := New()

	var (
		mu     sync.Mutex
		called []string
	)
	// Typed subscribers of siblingEvent: an untyped handler func would be an
	// any-event subscriber instead.
	record := func(name string) func(*siblingEvent) {
		return func(*siblingEvent) {
			mu.Lock()
			defer mu.Unlock()
			called = append(called, name)
		}
	}
	event.Subscribe(mgr, 0, record("first"))
	event.Subscribe(mgr, 0, record("second"))
	event.Subscribe(mgr, 0, func(*otherEvent) {})
	mgr.Subscribe(nil, 0, func(event.Event) {
		mu.Lock()
		defer mu.Unlock()
		called = append(called, "any")
	})

	recorded := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := called
		called = nil
		return out
	}

	require.Equal(t, 2, mgr.UnsubscribeAll(&siblingEvent{}),
		"UnsubscribeAll must return the number of subscribers it removed")
	mgr.Fire(&siblingEvent{})
	require.Equal(t, []string{"any"}, recorded(),
		"only the any-event subscriber is still subscribed to siblingEvent")

	require.Equal(t, 2, mgr.UnsubscribeAll(), "the remaining subscribers of all events must be removed")
	require.False(t, mgr.HasSubscriber())
	mgr.Fire(&otherEvent{})
	mgr.Fire(&siblingEvent{})
	require.Empty(t, recorded(), "no subscriber is left to be called")
}

// TestUnsubscribeIsIdempotent pins that the unsubscribe func can be called more
// than once and after UnsubscribeAll.
func TestUnsubscribeIsIdempotent(t *testing.T) {
	mgr := New()

	var calls atomic.Int32
	unsubscribe := event.Subscribe(mgr, 0, func(*siblingEvent) { calls.Add(1) })

	unsubscribe()
	unsubscribe()
	require.False(t, mgr.HasSubscriber(&siblingEvent{}),
		"the only subscriber of the type was removed")

	mgr.Fire(&siblingEvent{})
	require.Zero(t, calls.Load())

	// A stale unsubscribe of a subscriber UnsubscribeAll removed must be a no-op.
	unsubscribeSecond := event.Subscribe(mgr, 0, func(*siblingEvent) { calls.Add(1) })
	require.Equal(t, 1, mgr.UnsubscribeAll(&siblingEvent{}))
	require.NotPanics(t, unsubscribeSecond)
}

// TestWaitBlocksUntilSubscribersReturned pins Wait: the proxy waits for its
// subscribers at shutdown with it.
func TestWaitBlocksUntilSubscribersReturned(t *testing.T) {
	mgr := New()

	started := make(chan struct{})
	release := make(chan struct{})
	event.Subscribe(mgr, 0, func(*siblingEvent) {
		close(started)
		<-release
	})

	go mgr.Fire(&siblingEvent{})
	<-started

	waited := make(chan struct{})
	go func() {
		mgr.Wait()
		close(waited)
	}()

	select {
	case <-waited:
		t.Fatal("Wait returned while a subscriber was still running")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case <-waited:
	case <-time.After(inFlightProbeTimeout):
		t.Fatal("Wait did not return after the subscriber returned")
	}
}

// TestFireParallelRunsAfterHandlersLast pins FireParallel: subscribers first,
// then the after handlers.
func TestFireParallelRunsAfterHandlersLast(t *testing.T) {
	mgr := New()

	var (
		mu     sync.Mutex
		called []string
	)
	event.Subscribe(mgr, 0, func(*siblingEvent) {
		mu.Lock()
		defer mu.Unlock()
		called = append(called, "subscriber")
	})

	done := make(chan struct{})
	event.FireParallel(mgr, &siblingEvent{}, func(*siblingEvent) {
		mu.Lock()
		defer mu.Unlock()
		called = append(called, "after")
		close(done)
	})

	select {
	case <-done:
	case <-time.After(inFlightProbeTimeout):
		t.Fatal("the after handler never ran")
	}

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"subscriber", "after"}, called)
}

// TestSubscriberPanicIsRecoveredPerSubscriber pins recovery parity with the
// wrapped manager: a panicking subscriber is caught and the remaining
// subscribers of that event type still run.
func TestSubscriberPanicIsRecoveredPerSubscriber(t *testing.T) {
	mgr := New()

	var survivors atomic.Int32
	event.Subscribe(mgr, 1, func(*siblingEvent) { panic("subscriber panic") })
	event.Subscribe(mgr, 0, func(*siblingEvent) { survivors.Add(1) })

	require.NotPanics(t, func() { mgr.Fire(&siblingEvent{}) })
	require.Equal(t, int32(1), survivors.Load(),
		"a panicking subscriber must not cancel the remaining subscribers")
}

// TestSubscriberPanicPropagatesWithoutRecovery pins the other side of the
// parity: with recovery disabled the panic reaches the firing goroutine.
func TestSubscriberPanicPropagatesWithoutRecovery(t *testing.T) {
	mgr := New(WithRecoverPanic(false))

	event.Subscribe(mgr, 0, func(*siblingEvent) { panic("subscriber panic") })

	require.PanicsWithValue(t, "subscriber panic", func() { mgr.Fire(&siblingEvent{}) })
}

// TestConcurrentSubscribeFireUnsubscribeIsRaceFree exercises the interleavings
// the wrapping exists for: many subscribers of one event type, fired while
// other goroutines subscribe and unsubscribe. It is the assertion of
// `go test -race`; without the wrapping the same work races inside the wrapped
// manager.
func TestConcurrentSubscribeFireUnsubscribeIsRaceFree(t *testing.T) {
	mgr := New()

	const (
		goroutines = 8
		iterations = 200
	)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				unsubscribe := mgr.Subscribe(&siblingEvent{}, i%3, func(event.Event) {})
				unsubscribeAny := mgr.Subscribe(nil, 0, func(event.Event) {})
				mgr.Fire(&siblingEvent{})
				if i%2 == 0 {
					unsubscribe()
				}
				if i%3 == 0 {
					unsubscribeAny()
				}
				if i%7 == 0 {
					mgr.UnsubscribeAll(&siblingEvent{})
				}
			}
		}(g)
	}
	wg.Wait()
}

// otherEvent is a second event type for the tests that need one.
type otherEvent struct{}
