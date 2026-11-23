package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/forge"
	"go.minekube.com/gate/pkg/edition/java/proto/codec"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/loopback"
	"go.minekube.com/gate/pkg/util/uuid"
)

// The backend deliberately waits for ClientHello before sending JoinGame. If
// initial connection establishment blocks the client read loop, Gate relays the
// ServerHello but never reads the reply, so neither side can finish the join.
func TestLegacyForgeInitialConnectionReadsFMLBeforeJoinGame(t *testing.T) {
	allocator, err := loopback.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		protocol proto.Protocol
		marker   string
		finish   string
	}{
		{"1.7.2_unmarked", version.Minecraft_1_7_2.Protocol, "", "join"},
		{"1.7.10_unmarked", version.Minecraft_1_7_6.Protocol, "", "join"},
		{"1.8", version.Minecraft_1_8.Protocol, forge.HandshakeHostnameToken, "join"},
		{"1.12.2", version.Minecraft_1_12_2.Protocol, forge.HandshakeHostnameToken, "join"},
		{"1.12.2_backend_reject", version.Minecraft_1_12_2.Protocol, forge.HandshakeHostnameToken, "reject"},
		{"1.12.2_client_cancel", version.Minecraft_1_12_2.Protocol, forge.HandshakeHostnameToken, "cancel"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend, err := allocator.Listen()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = backend.Close() })

			cfg := config.DefaultConfig
			cfg.OnlineMode = false
			cfg.Forwarding.Mode = config.NoneForwardingMode
			cfg.Compression.Threshold = -1
			cfg.Servers = map[string]string{"forge": backend.Addr().String()}
			cfg.Try = []string{"forge"}
			p, err := New(Options{Config: &cfg, EventMgr: event.New()})
			if err != nil {
				t.Fatal(err)
			}
			if err := p.init(); err != nil {
				t.Fatal(err)
			}

			client, proxySide := net.Pipe()
			proxyDone := make(chan struct{})
			go func() {
				defer close(proxyDone)
				p.HandleConn(proxySide)
			}()
			t.Cleanup(func() {
				_ = client.Close()
				_ = proxySide.Close()
				select {
				case <-proxyDone:
				case <-time.After(5 * time.Second):
					t.Error("client handler did not exit after disconnect")
				}
			})
			if err := client.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}

			backendDone := make(chan error, 1)
			releaseBackend := make(chan struct{})
			t.Cleanup(func() { close(releaseBackend) })
			go func() {
				conn, err := backend.Accept()
				if err != nil {
					backendDone <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				backendDone <- legacyForgeBackendJoin(conn, tc.protocol, tc.finish)
				<-releaseBackend
			}()

			enc := codec.NewEncoder(client, proto.ServerBound, logr.Discard())
			enc.SetProtocol(tc.protocol)
			if _, err := enc.WritePacket(&packet.Handshake{
				ProtocolVersion: int(tc.protocol), ServerAddress: "localhost" + tc.marker,
				Port: 25565, NextStatus: 2,
			}); err != nil {
				t.Fatal(err)
			}
			enc.SetState(state.Login)
			if _, err := enc.WritePacket(&packet.ServerLogin{Username: "LegacyForge"}); err != nil {
				t.Fatal(err)
			}
			dec := codec.NewDecoder(client, proto.ClientBound, logr.Discard())
			dec.SetProtocol(tc.protocol)
			dec.SetState(state.Login)
			pc, err := dec.Decode()
			if err != nil {
				t.Fatalf("read LoginSuccess: %v", err)
			}
			if _, ok := pc.Packet.(*packet.ServerLoginSuccess); !ok {
				t.Fatalf("got %T, want LoginSuccess", pc.Packet)
			}
			enc.SetState(state.Play)
			dec.SetState(state.Play)
			pc, err = dec.Decode()
			if err != nil {
				t.Fatalf("read FML ServerHello: %v", err)
			}
			hello, ok := pc.Packet.(*plugin.Message)
			if !ok || hello.Channel != forge.LegacyHandshakeChannel || !bytes.Equal(hello.Data, []byte{0, 2}) {
				t.Fatalf("got %#v, want FML ServerHello", pc.Packet)
			}

			if tc.finish != "cancel" {
				if _, err := enc.WritePacket(&plugin.Message{
					Channel: forge.LegacyHandshakeChannel, Data: []byte{1, 2},
				}); err != nil {
					t.Fatalf("write FML ClientHello before JoinGame: %v", err)
				}
				pc, err = dec.Decode()
				if err != nil {
					t.Fatalf("read backend result after FML reply: %v", err)
				}
				if tc.finish == "reject" {
					disconnect, ok := pc.Packet.(*packet.Disconnect)
					if !ok || disconnect.Reason == nil || !bytes.Contains(disconnect.Reason.JSON, []byte("FML rejected")) {
						t.Fatalf("got %#v, want backend FML rejection", pc.Packet)
					}
				} else if _, ok := pc.Packet.(*packet.JoinGame); !ok {
					t.Fatalf("got %T, want JoinGame", pc.Packet)
				}
			}
			if err := client.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-backendDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("backend did not finish the initial connection")
			}
			deadline := time.Now().Add(5 * time.Second)
			for p.PlayerCount() != 0 {
				if time.Now().After(deadline) {
					t.Fatal("player registration was not released after disconnect")
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}

func legacyForgeBackendJoin(conn net.Conn, protocol proto.Protocol, finish string) error {
	dec := codec.NewDecoder(conn, proto.ServerBound, logr.Discard())
	dec.SetProtocol(protocol)
	if _, err := dec.Decode(); err != nil {
		return fmt.Errorf("backend handshake: %w", err)
	}
	dec.SetState(state.Login)
	pc, err := dec.Decode()
	if err != nil {
		return fmt.Errorf("backend LoginStart: %w", err)
	}
	login, ok := pc.Packet.(*packet.ServerLogin)
	if !ok {
		return fmt.Errorf("backend got %T, want LoginStart", pc.Packet)
	}
	enc := codec.NewEncoder(conn, proto.ClientBound, logr.Discard())
	enc.SetProtocol(protocol)
	enc.SetState(state.Login)
	if _, err := enc.WritePacket(&packet.ServerLoginSuccess{
		UUID: uuid.OfflinePlayerUUID(login.Username), Username: login.Username,
	}); err != nil {
		return err
	}
	enc.SetState(state.Play)
	dec.SetState(state.Play)
	if _, err := enc.WritePacket(&plugin.Message{Channel: forge.LegacyHandshakeChannel, Data: []byte{0, 2}}); err != nil {
		return err
	}
	pc, err = dec.Decode()
	if finish == "cancel" {
		if !errors.Is(err, io.EOF) {
			return fmt.Errorf("backend read after client cancellation = (%#v, %v), want EOF", pc, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("backend FML reply before JoinGame: %w", err)
	}
	reply, ok := pc.Packet.(*plugin.Message)
	if !ok || reply.Channel != forge.LegacyHandshakeChannel || !bytes.Equal(reply.Data, []byte{1, 2}) {
		return fmt.Errorf("backend got %#v, want FML ClientHello", pc.Packet)
	}
	if finish == "reject" {
		_, err = enc.WritePacket(packet.NewDisconnect(&component.Text{Content: "FML rejected"}, protocol, states.PlayState))
		return err
	}
	levelType := "default"
	_, err = enc.WritePacket(&packet.JoinGame{EntityID: 42, MaxPlayers: 20, LevelType: &levelType})
	return err
}
