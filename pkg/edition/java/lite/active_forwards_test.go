package lite

import (
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/util/netutil"
)

func TestActiveForwardTrackerLifecycleAndCopies(t *testing.T) {
	tracker := newActiveForwardTracker()
	clientIP := netip.MustParseAddr("203.0.113.10")
	backendAddr := netutil.NewAddr("127.0.0.1:25565", "tcp")

	f := ActiveForward{
		ConnectionID: "lfwd-1",
		ClientIP:     clientIP,
		BackendAddr:  backendAddr,
		Host:         "play.example.com",
		RouteID:      "0:*.example.com",
		StartedAt:    time.Now(),
	}
	tracker.add(f)

	byID, ok := tracker.get("lfwd-1")
	require.True(t, ok)
	assert.Equal(t, f.ConnectionID, byID.ConnectionID)
	assert.Equal(t, f.ClientIP, byID.ClientIP)
	assert.Equal(t, f.BackendAddr.String(), byID.BackendAddr.String())
	assert.NotSame(t, backendAddr, byID.BackendAddr)

	byIP := tracker.listByClientIP(clientIP)
	require.Len(t, byIP, 1)
	assert.Equal(t, "lfwd-1", byIP[0].ConnectionID)
	assert.NotSame(t, backendAddr, byIP[0].BackendAddr)

	removed, ok := tracker.remove("lfwd-1")
	require.True(t, ok)
	assert.Equal(t, "lfwd-1", removed.ConnectionID)

	_, ok = tracker.get("lfwd-1")
	assert.False(t, ok)
	assert.Empty(t, tracker.listByClientIP(clientIP))
}

func TestActiveForwardTrackerConcurrentAccess(t *testing.T) {
	tracker := newActiveForwardTracker()
	clientIP := netip.MustParseAddr("203.0.113.20")

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := tracker.nextConnectionID()
			tracker.add(ActiveForward{
				ConnectionID: id,
				ClientIP:     clientIP,
				BackendAddr:  netutil.NewAddr("127.0.0.1:25565", "tcp"),
				Host:         "play.example.com",
				RouteID:      "0:play.example.com",
				StartedAt:    time.Now(),
			})
			_, _ = tracker.get(id)
			_ = tracker.listByClientIP(clientIP)
			tracker.remove(id)
		}(i)
	}
	wg.Wait()

	assert.Empty(t, tracker.listByClientIP(clientIP))
}
