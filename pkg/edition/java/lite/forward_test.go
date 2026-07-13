package lite

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/gate/proto"
	"golang.org/x/sync/singleflight"
)

func TestPingCacheDoesNotExtendExpiryOnHotGet(t *testing.T) {
	ResetPingCache()
	t.Cleanup(ResetPingCache)

	key := pingKey{backendAddr: "backend.example:25565", protocol: proto.Protocol(765)}
	item := pingCache.Set(key, &pingResult{}, time.Hour)
	expiresAt := item.ExpiresAt()

	for range 100 {
		require.Same(t, item, pingCache.Get(key))
	}

	require.Equal(t, expiresAt, item.ExpiresAt(), "status reads must not extend cache expiry")
}

func TestPingCacheLoaderSuppressesConcurrentMisses(t *testing.T) {
	cache := ttlcache.New[pingKey, *pingResult]()
	key := pingKey{backendAddr: "backend.example:25565", protocol: proto.Protocol(765)}

	var loads atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	loader := withLoader(new(singleflight.Group), time.Hour, func(pingKey) *pingResult {
		loads.Add(1)
		startOnce.Do(func() { close(started) })
		<-release
		return &pingResult{}
	})

	const requests = 32
	results := make(chan *ttlcache.Item[pingKey, *pingResult], requests+1)
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		results <- cache.Get(key, loader)
	}()
	<-started

	var ready sync.WaitGroup
	ready.Add(requests)
	var done sync.WaitGroup
	done.Add(requests)
	start := make(chan struct{})
	for range requests {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			results <- cache.Get(key, loader)
		}()
	}
	ready.Wait()
	close(start)
	close(release)
	<-firstDone
	done.Wait()

	for range requests + 1 {
		require.NotNil(t, <-results)
	}
	require.Equal(t, int32(1), loads.Load())
}

func TestPingCacheSeparatesProtocols(t *testing.T) {
	cache := ttlcache.New[pingKey, *pingResult]()
	backendAddr := "backend.example:25565"
	legacy := &pingResult{res: &packet.StatusResponse{Status: "legacy"}}
	modern := &pingResult{res: &packet.StatusResponse{Status: "modern"}}

	cache.Set(pingKey{backendAddr: backendAddr, protocol: proto.Protocol(47)}, legacy, time.Hour)
	cache.Set(pingKey{backendAddr: backendAddr, protocol: proto.Protocol(765)}, modern, time.Hour)

	require.Same(t, legacy, cache.Get(pingKey{backendAddr: backendAddr, protocol: proto.Protocol(47)}).Value())
	require.Same(t, modern, cache.Get(pingKey{backendAddr: backendAddr, protocol: proto.Protocol(765)}).Value())
}

func TestResetPingCacheInvalidatesAllBackendsAndProtocols(t *testing.T) {
	ResetPingCache()
	t.Cleanup(ResetPingCache)

	keys := []pingKey{
		{backendAddr: "one.example:25565", protocol: proto.Protocol(47)},
		{backendAddr: "one.example:25565", protocol: proto.Protocol(765)},
		{backendAddr: "two.example:25565", protocol: proto.Protocol(765)},
	}
	for _, key := range keys {
		pingCache.Set(key, &pingResult{}, time.Hour)
	}

	ResetPingCache()

	for _, key := range keys {
		require.Nil(t, pingCache.Get(key))
	}
}

// TestDecodeStatusResponse_WithErrDecoderLeftBytes tests that decodeStatusResponse
// properly handles ErrDecoderLeftBytes error (from BetterCompatibilityChecker mod)
// This test verifies the fix for issue #297: "Status/ping fails when server has the BetterCompatibilityChecker mod"
func TestDecodeStatusResponse_WithErrDecoderLeftBytes(t *testing.T) {
	// Create a mock decoder that returns ErrDecoderLeftBytes
	// This simulates the scenario from issue #297 where BetterCompatibilityChecker mod
	// adds extra data to status response packets
	mockDecoder := &mockDecoder{
		packetCtx: &proto.PacketContext{
			Packet: &packet.StatusResponse{
				Status: `{"version":{"name":"Test","protocol":754},"players":{"online":5,"max":20},"description":"Test Server"}`,
			},
		},
		err: proto.ErrDecoderLeftBytes, // This is the error from BetterCompatibilityChecker (issue #297)
	}

	// Test that decodeStatusResponse handles the error correctly
	result, err := decodeStatusResponse(mockDecoder)

	// Should succeed despite ErrDecoderLeftBytes (fixing issue #297)
	if err != nil {
		t.Errorf("decodeStatusResponse should ignore ErrDecoderLeftBytes (issue #297), got error: %v", err)
	}

	require.NotNil(t, result, "decodeStatusResponse returned nil result")

	// Verify the status response was properly decoded
	expectedStatus := `{"version":{"name":"Test","protocol":754},"players":{"online":5,"max":20},"description":"Test Server"}`
	if result.Status != expectedStatus {
		t.Errorf("Expected status %q, got %q", expectedStatus, result.Status)
	}
}

// TestDecodeStatusResponse_WithOtherError tests that other errors are still propagated
func TestDecodeStatusResponse_WithOtherError(t *testing.T) {
	// Create a mock decoder that returns a different error
	otherErr := errors.New("connection timeout")
	mockDecoder := &mockDecoder{
		err: otherErr,
	}

	// Test that other errors are still propagated
	result, err := decodeStatusResponse(mockDecoder)

	// Should fail with the other error
	if err == nil {
		t.Error("decodeStatusResponse should propagate other errors")
	}

	if result != nil {
		t.Error("decodeStatusResponse should return nil result on error")
	}

	// Verify the error is wrapped correctly
	if !errors.Is(err, otherErr) {
		t.Errorf("Expected error to contain %v, got %v", otherErr, err)
	}
}

// TestDecodeStatusResponse_Success tests normal successful decoding
func TestDecodeStatusResponse_Success(t *testing.T) {
	// Create a mock decoder that succeeds
	mockDecoder := &mockDecoder{
		packetCtx: &proto.PacketContext{
			Packet: &packet.StatusResponse{
				Status: `{"version":{"name":"Normal","protocol":754},"players":{"online":10,"max":50}}`,
			},
		},
		err: nil, // No error
	}

	// Test successful decoding
	result, err := decodeStatusResponse(mockDecoder)

	// Should succeed
	if err != nil {
		t.Errorf("decodeStatusResponse should succeed, got error: %v", err)
	}

	require.NotNil(t, result, "decodeStatusResponse returned nil result")

	// Verify the status response
	expectedStatus := `{"version":{"name":"Normal","protocol":754},"players":{"online":10,"max":50}}`
	if result.Status != expectedStatus {
		t.Errorf("Expected status %q, got %q", expectedStatus, result.Status)
	}
}

// TestDecodeStatusResponse_WrongPacketType tests handling of unexpected packet types
func TestDecodeStatusResponse_WrongPacketType(t *testing.T) {
	// Create a mock decoder that returns wrong packet type
	mockDecoder := &mockDecoder{
		packetCtx: &proto.PacketContext{
			Packet: &packet.StatusRequest{}, // Wrong type!
		},
		err: nil,
	}

	// Test that wrong packet type is handled
	result, err := decodeStatusResponse(mockDecoder)

	// Should fail
	if err == nil {
		t.Error("decodeStatusResponse should fail with wrong packet type")
	}

	if result != nil {
		t.Error("decodeStatusResponse should return nil result on wrong packet type")
	}
}

// mockDecoder implements the statusDecoder interface for testing
type mockDecoder struct {
	packetCtx *proto.PacketContext
	err       error
}

func (m *mockDecoder) Decode() (*proto.PacketContext, error) {
	return m.packetCtx, m.err
}

func Test_substituteBackendParams(t *testing.T) {
	tests := []struct {
		name     string
		template string
		groups   []string
		want     string
	}{
		{
			name:     "single parameter",
			template: "$1.servers.svc:25565",
			groups:   []string{"abc"},
			want:     "abc.servers.svc:25565",
		},
		{
			name:     "multiple parameters",
			template: "$1-$2.servers.svc:25565",
			groups:   []string{"abc", "def"},
			want:     "abc-def.servers.svc:25565",
		},
		{
			name:     "three parameters",
			template: "$1.$2.$3.servers.svc:25565",
			groups:   []string{"a", "b", "c"},
			want:     "a.b.c.servers.svc:25565",
		},
		{
			name:     "parameter in middle",
			template: "prefix-$1-suffix:25565",
			groups:   []string{"middle"},
			want:     "prefix-middle-suffix:25565",
		},
		{
			name:     "multiple same parameter",
			template: "$1.$1.servers.svc:25565",
			groups:   []string{"abc"},
			want:     "abc.abc.servers.svc:25565",
		},
		{
			name:     "no groups",
			template: "$1.servers.svc:25565",
			groups:   []string{},
			want:     "$1.servers.svc:25565",
		},
		{
			name:     "no parameters in template",
			template: "static.backend:25565",
			groups:   []string{"abc", "def"},
			want:     "static.backend:25565",
		},
		{
			name:     "out of range parameter",
			template: "$1.$99.servers.svc:25565",
			groups:   []string{"abc"},
			want:     "abc.$99.servers.svc:25565",
		},
		{
			name:     "parameter index beyond groups",
			template: "$1.$2.$3.servers.svc:25565",
			groups:   []string{"abc", "def"},
			want:     "abc.def.$3.servers.svc:25565",
		},
		{
			name:     "empty group value",
			template: "$1.servers.svc:25565",
			groups:   []string{""},
			want:     ".servers.svc:25565",
		},
		{
			name:     "real world example",
			template: "$1.servers.svc:25565",
			groups:   []string{"abc"},
			want:     "abc.servers.svc:25565",
		},
		{
			name:     "parameter with port",
			template: "$1.servers.svc:$2",
			groups:   []string{"abc", "25565"},
			want:     "abc.servers.svc:25565",
		},
		{
			name:     "higher index first",
			template: "$2.$1.servers.svc:25565",
			groups:   []string{"abc", "def"},
			want:     "def.abc.servers.svc:25565",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := substituteBackendParams(tt.template, tt.groups)
			if got != tt.want {
				t.Errorf("substituteBackendParams(%q, %v) = %q, want %q", tt.template, tt.groups, got, tt.want)
			}
		})
	}
}
