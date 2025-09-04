package lite

import (
	"errors"
	"testing"

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

	if result == nil {
		t.Fatal("decodeStatusResponse returned nil result")
	}

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

	if result == nil {
		t.Fatal("decodeStatusResponse returned nil result")
	}

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

// mockDecoder implements the StatusDecoder interface for testing
type mockDecoder struct {
	packetCtx *proto.PacketContext
	err       error
}

func (m *mockDecoder) Decode() (*proto.PacketContext, error) {
	return m.packetCtx, m.err
}

