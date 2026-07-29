package gatehost

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/builtin/wasm/runtime/dispatch"
	"go.minekube.com/gate/internal/builtin/wasm/runtime/resources"
	"go.minekube.com/gate/pkg/command"
)

func TestCommandRegistrationAliasesAndCleanup(t *testing.T) {
	t.Parallel()

	manager := &command.Manager{}
	host := &Host{
		dispatch:       dispatch.NewHost(resources.NewTable("commands", 32)),
		commandManager: manager,
		timers:         make(map[*ownedTimer]struct{}),
		timerLimit:     4,
	}
	result, err := host.registerCommand(
		dispatch.Operation{Identity: "fixture#wasm-register-command"},
		[]any{
			"Hello",
			[]string{"Hi"},
			func(*command.Context) error { return nil },
		},
	)
	require.NoError(t, err)
	require.True(t, manager.Has("hello"))
	require.True(t, manager.Has("hi"))

	result[0].(func())()
	require.False(t, manager.Has("hello"))
	require.False(t, manager.Has("hi"))
}

func TestOneShotAndRecurringTimersDoNotOverlapAndCloseCleanly(t *testing.T) {
	t.Parallel()

	host := &Host{
		context:    context.Background(),
		dispatch:   dispatch.NewHost(resources.NewTable("timers", 32)),
		timers:     make(map[*ownedTimer]struct{}),
		timerLimit: 2,
	}
	oneShot := make(chan struct{}, 1)
	_, err := host.scheduleTimer(
		dispatch.Operation{Identity: "fixture#wasm-after"},
		[]any{int64(time.Millisecond), func() error {
			oneShot <- struct{}{}
			return nil
		}},
		false,
	)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return len(oneShot) == 1
	}, time.Second, time.Millisecond)

	var active atomic.Int32
	var maximum atomic.Int32
	release := make(chan struct{}, 2)
	result, err := host.scheduleTimer(
		dispatch.Operation{Identity: "fixture#wasm-every"},
		[]any{int64(time.Millisecond), func() error {
			current := active.Add(1)
			if current > maximum.Load() {
				maximum.Store(current)
			}
			<-release
			active.Add(-1)
			return nil
		}},
		true,
	)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return active.Load() == 1
	}, time.Second, time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	require.Equal(t, int32(1), maximum.Load())
	release <- struct{}{}
	result[0].(func())()
	require.NoError(t, host.Close())
}
