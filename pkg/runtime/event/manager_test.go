package event

import (
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type myEvent struct{ s string }

func TestTypeOf(t *testing.T) {
	assert.Equal(t, TypeOf(&myEvent{}), reflect.TypeOf(&myEvent{}))
	assert.Equal(t, TypeOf(reflect.TypeOf(&myEvent{})), reflect.TypeOf(&myEvent{}))
	assert.NotEqual(t, TypeOf(reflect.TypeOf(myEvent{})), reflect.TypeOf(&myEvent{}))
}

func TestPriorityAndCorrectType(t *testing.T) {
	m := New(logr.Discard())
	require.False(t, m.HasSubscribers(&myEvent{}))

	m.Subscribe(TypeOf(&myEvent{}), -1, func(e Event) {
		ev := e.(*myEvent)
		ev.s += "c"
	})
	m.Subscribe(&myEvent{}, 1, func(e Event) {
		ev := e.(*myEvent)
		ev.s += "a"
	})
	m.Subscribe(TypeOf(myEvent{}), 0, func(e Event) {
		ev := e.(myEvent)
		ev.s += "d"
	})
	Subscribe(m, 0, func(ev *myEvent) {
		ev.s += "b"
	})

	var noPtr bool
	m.Subscribe(myEvent{}, 2, func(e Event) {
		_ = e.(myEvent)
		noPtr = true
	})

	e := &myEvent{s: "_"}
	m.Fire(e)
	require.False(t, noPtr)
	require.Equal(t, "_abc", e.s)
	require.True(t, m.HasSubscribers(&myEvent{}))
}
