package codec

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/go-logr/logr"

	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// TestLoginHelloEncodingAcrossProtocols encodes the login-phase "hello"
// (EncryptionRequest) exactly the way the proxy writes it to a client and parses
// the frame back with the modern (>= 1.8) layout, which is what a real client
// does.
//
// Regression (2026-09-18, Minecraft 26.3 / protocol 777): for a protocol that was
// missing from the version table the packet registry fell back to the minimum
// version (1.7.2), so the hello was written with a one-byte public key length
// (0xa2) instead of the varint (0xa2 0x01) and without the shouldAuthenticate
// bool. Every 26.3 client died with
// "Failed to decode packet 'clientbound/minecraft:hello'"
// (162-byte key, frame 170 instead of 172, declared key length 6178 with only
// 166 bytes left).
func TestLoginHelloEncodingAcrossProtocols(t *testing.T) {
	tests := []struct {
		name     string
		protocol proto.Protocol
		wantLen  int
	}{
		{"1.21.11 (774)", version.Minecraft_1_21_11.Protocol, 172},
		{"26.2 (776)", version.Minecraft_26_2.Protocol, 172},
		{"26.3 (777)", 777, 172},
		{"unknown newer protocol (778) keeps the newest known layout", 778, 172},
		{"far newer protocol (900) keeps the newest known layout", 900, 172},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := encodeLoginHello(t, tt.protocol)

			if len(frame) != tt.wantLen {
				t.Fatalf("login hello frame = %d bytes, want %d (pre-1.8 layout is 170)", len(frame), tt.wantLen)
			}

			// Packet id 0x01 = EncryptionRequest in the login state, then the empty
			// server id, then the varint-encoded public key length.
			if frame[0] != 0x01 {
				t.Fatalf("packet id = %#x, want 0x01 (login EncryptionRequest)", frame[0])
			}
			if frame[1] != 0x00 {
				t.Fatalf("server id length = %#x, want 0x00", frame[1])
			}
			keyLen, n := readVarInt(t, frame[2:])
			if keyLen != 162 {
				t.Fatalf("public key length = %d, want 162 (two-byte varint 0xa2 0x01)", keyLen)
			}
			if n != 2 {
				t.Fatalf("public key length used %d bytes, want 2 - one byte means the pre-1.8 layout", n)
			}

			// token length + token + shouldAuthenticate bool must fill the rest.
			tokenLen, tn := readVarInt(t, frame[2+n+int(keyLen):])
			if tokenLen != 4 {
				t.Fatalf("verify token length = %d, want 4", tokenLen)
			}
			trailing := frame[2+n+int(keyLen)+tn+int(tokenLen):]
			if len(trailing) != 1 {
				t.Fatalf("trailing bytes after verify token = %d, want exactly the 1-byte shouldAuthenticate bool", len(trailing))
			}
			if trailing[0] != 0x01 {
				t.Fatalf("shouldAuthenticate = %#x, want 0x01 (online mode)", trailing[0])
			}
		})
	}
}

// encodeLoginHello writes a login-state clientbound EncryptionRequest for the
// given client protocol and returns the frame body (packet id + data).
func encodeLoginHello(t *testing.T, protocol proto.Protocol) []byte {
	t.Helper()

	var buf bytes.Buffer
	enc := NewEncoder(&buf, proto.ClientBound, logr.Discard())
	enc.SetState(state.Login)
	enc.SetProtocol(protocol)

	key := make([]byte, 162)
	key[0], key[1], key[2] = 0x30, 0x81, 0x9f // realistic DER prefix

	if _, err := enc.WritePacket(&packet.EncryptionRequest{
		ServerID:    "",
		PublicKey:   key,
		VerifyToken: []byte{0xde, 0xad, 0xbe, 0xef},
	}); err != nil {
		t.Fatalf("encode EncryptionRequest for protocol %d: %v", protocol, err)
	}

	frame := buf.Bytes()
	length, n := readVarInt(t, frame)
	if length != len(frame)-n {
		t.Fatalf("frame length prefix = %d, body = %d", length, len(frame)-n)
	}
	return frame[n:]
}

func readVarInt(t *testing.T, b []byte) (int, int) {
	t.Helper()
	var (
		value int
		shift uint
	)
	for i, by := range b {
		value |= int(by&0x7F) << shift
		if by&0x80 == 0 {
			return value, i + 1
		}
		shift += 7
		if shift > 35 {
			t.Fatalf("varint too long: %s", fmt.Sprint(b))
		}
	}
	t.Fatalf("truncated varint: %v", b)
	return 0, 0
}
