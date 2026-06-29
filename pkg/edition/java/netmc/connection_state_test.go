package netmc

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

func TestSetOutboundStateReleasesPlayPacketQueue(t *testing.T) {
	base := &recordingConn{}
	conn, _ := NewMinecraftConn(
		context.Background(),
		base,
		proto.ServerBound,
		time.Second,
		time.Second,
		-1,
		nil,
	)
	conn.SetProtocol(version.Minecraft_1_21_6.Protocol)
	conn.SetState(state.Config)

	err := conn.BufferPacket(&packet.HeaderAndFooter{
		Header: *chat.FromComponent(new(component.Text)),
		Footer: *chat.FromComponent(new(component.Text)),
	})
	if err != nil {
		t.Fatalf("buffer play packet in config state: %v", err)
	}
	if base.Len() != 0 {
		t.Fatalf("play packet was written before outbound play state, wrote %d bytes", base.Len())
	}

	conn.SetOutboundState(state.Play)

	if base.Len() == 0 {
		t.Fatal("queued play packet was not released when outbound state switched to play")
	}
	if conn.State() != state.Config {
		t.Fatalf("SetOutboundState changed inbound state: got %v, want %v", conn.State(), state.Config)
	}
}

type recordingConn struct {
	bytes.Buffer
}

func (c *recordingConn) Read([]byte) (int, error)         { select {} }
func (c *recordingConn) Close() error                     { return nil }
func (c *recordingConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *recordingConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *recordingConn) SetDeadline(time.Time) error      { return nil }
func (c *recordingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *recordingConn) SetWriteDeadline(time.Time) error { return nil }
