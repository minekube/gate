package lite

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/gate/proto"
)

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
