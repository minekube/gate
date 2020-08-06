package event

import (
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

type myEvent struct{ s string }

func TestTypeOf(t *testing.T) {
	var e ***myEvent
	require.Equal(t, TypeOf(e), reflect.TypeOf(myEvent{}))
}

func Test(t *testing.T) {
	m := NewManager()
	m.Subscribe(TypeOf(myEvent{}), -1, func(e Event) {
		ev := e.(*myEvent)
		ev.s += "c"
	})
	m.Subscribe(TypeOf(myEvent{}), 1, func(e Event) {
		ev := e.(*myEvent)
		ev.s += "a"
	})
	m.Subscribe(TypeOf(myEvent{}), 0, func(e Event) {
		ev := e.(*myEvent)
		ev.s += "b"
	})
	e := &myEvent{s: "_"}
	m.Fire(e)
	require.Equal(t, "_abc", e.s)
}
