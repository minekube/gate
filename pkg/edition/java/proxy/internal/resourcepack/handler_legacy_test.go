package resourcepack

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

const legacyTestTimeout = 3 * time.Second

type legacyTestPlayer struct {
	testPlayer
	protocol proto.Protocol
	writes   chan proto.Packet
	write    func(proto.Packet) error
}

func (p *legacyTestPlayer) Protocol() proto.Protocol { return p.protocol }
func (p *legacyTestPlayer) WritePacket(packet proto.Packet) error {
	if p.write != nil {
		return p.write(packet)
	}
	p.writes <- packet
	return nil
}

type legacyTestBackend struct {
	writes chan proto.Packet
	write  func(proto.Packet) error
}

func (b *legacyTestBackend) WritePacket(packet proto.Packet) error {
	if b.write != nil {
		return b.write(packet)
	}
	b.writes <- packet
	return nil
}

func legacyCall[T any](t *testing.T, fn func() T) T {
	t.Helper()
	done := make(chan T, 1)
	go func() { done <- fn() }()
	select {
	case result := <-done:
		return result
	case <-time.After(legacyTestTimeout):
		t.Fatal("resource pack operation timed out")
		var zero T
		return zero
	}
}

func legacyPacket(t *testing.T, packets <-chan proto.Packet) proto.Packet {
	t.Helper()
	select {
	case p := <-packets:
		return p
	case <-time.After(legacyTestTimeout):
		t.Fatal("timed out waiting for resource pack packet")
		return nil
	}
}

func legacyResponse(t *testing.T, h Handler, status packet.ResponseStatus) bool {
	t.Helper()
	type result struct {
		handled bool
		err     error
	}
	got := legacyCall(t, func() result {
		handled, err := h.OnResourcePackResponse(&ResponseBundle{Status: status})
		return result{handled, err}
	})
	if got.err != nil {
		t.Fatalf("response %v: %v", status, got.err)
	}
	return got.handled
}

func assertLegacyRequest(t *testing.T, packets <-chan proto.Packet, url string, required bool) {
	t.Helper()
	p := legacyPacket(t, packets)
	req, ok := p.(*packet.ResourcePackRequest)
	if !ok || req.URL != url || req.Required != required {
		t.Fatalf("request = %#v, want URL %q, required %v", p, url, required)
	}
}

func assertLegacyStatus(t *testing.T, packets <-chan proto.Packet, status packet.ResponseStatus) {
	t.Helper()
	p := legacyPacket(t, packets)
	response, ok := p.(*packet.ResourcePackResponse)
	if !ok || response.Status != status {
		t.Fatalf("response = %#v, want status %v", p, status)
	}
}

func assertNoLegacyPacket(t *testing.T, packets <-chan proto.Packet) {
	t.Helper()
	select {
	case p := <-packets:
		t.Fatalf("unexpected packet: %#v", p)
	default:
	}
}

func TestLegacyFirstResourcePackRequest(t *testing.T) {
	for _, required := range []bool{false, true} {
		t.Run(fmt.Sprintf("required=%v", required), func(t *testing.T) {
			player := &legacyTestPlayer{protocol: version.Minecraft_1_20.Protocol, writes: make(chan proto.Packet, 1)}
			h := NewHandler(player, event.Nop)
			info := &Info{URL: "https://example.invalid/pack.zip", ShouldForce: required}
			if err := legacyCall(t, func() error { return h.QueueResourcePack(info) }); err != nil {
				t.Fatal(err)
			}
			assertLegacyRequest(t, player.writes, info.URL, required)
			if h.FirstPendingPack() != nil {
				t.Fatal("unanswered pack is pending")
			}
		})
	}
}

func TestLegacyResourcePackResponsesAndFIFO(t *testing.T) {
	player := &legacyTestPlayer{protocol: version.Minecraft_1_20.Protocol, writes: make(chan proto.Packet, 3)}
	backend := &legacyTestBackend{writes: make(chan proto.Packet, 4)}
	h := newLegacy117Handler(player, event.Nop)
	first := &Info{URL: "https://example.invalid/one.zip", Origin: DownstreamServerOrigin}
	second := &Info{URL: "https://example.invalid/two.zip", Origin: DownstreamServerOrigin}
	for _, info := range []*Info{first, second} {
		if err := legacyCall(t, func() error { return h.QueueResourcePackFromBackend(info, backend) }); err != nil {
			t.Fatal(err)
		}
	}
	assertLegacyRequest(t, player.writes, first.URL, false)
	assertNoLegacyPacket(t, player.writes)

	if !legacyResponse(t, h, AcceptedResponseStatus) || h.FirstPendingPack() != first {
		t.Fatal("accepted response was not handled or did not mark first pack pending")
	}
	assertLegacyStatus(t, backend.writes, AcceptedResponseStatus)
	assertNoLegacyPacket(t, player.writes)
	if !legacyResponse(t, h, SuccessfulResponseStatus) || h.FirstAppliedPack() != first || h.FirstPendingPack() != nil {
		t.Fatal("successful response was not handled or did not apply first pack")
	}
	assertLegacyStatus(t, backend.writes, SuccessfulResponseStatus)
	assertLegacyRequest(t, player.writes, second.URL, false)
	assertNoLegacyPacket(t, player.writes)
	legacyResponse(t, h, FailedDownloadResponseStatus)
	assertLegacyStatus(t, backend.writes, FailedDownloadResponseStatus)
	if h.FirstAppliedPack() != first || h.FirstPendingPack() != nil {
		t.Fatal("failed download changed applied pack or left a pending pack")
	}
}

func TestLegacyUntrackedAndDuplicateResponses(t *testing.T) {
	player := &legacyTestPlayer{protocol: version.Minecraft_1_20.Protocol, writes: make(chan proto.Packet, 1)}
	backend := &legacyTestBackend{writes: make(chan proto.Packet, 3)}
	h := newLegacy117Handler(player, event.Nop)
	for _, status := range []packet.ResponseStatus{AcceptedResponseStatus, SuccessfulResponseStatus, DeclinedResponseStatus} {
		if !legacyResponse(t, h, status) {
			t.Fatalf("untracked status %v was forwarded", status)
		}
	}
	first := &Info{ID: uuid.New(), URL: "https://example.invalid/one.zip", Origin: DownstreamServerOrigin}
	if err := h.QueueResourcePackFromBackend(first, backend); err != nil {
		t.Fatal(err)
	}
	assertLegacyRequest(t, player.writes, first.URL, false)
	mismatched := legacyCall(t, func() bool {
		handled, err := h.OnResourcePackResponse(&ResponseBundle{ID: uuid.New(), Status: SuccessfulResponseStatus})
		return handled && err == nil
	})
	if !mismatched || h.FirstAppliedPack() != nil {
		t.Fatal("mismatched response changed the active pack")
	}
	legacyResponse(t, h, AcceptedResponseStatus)
	assertLegacyStatus(t, backend.writes, AcceptedResponseStatus)
	legacyResponse(t, h, AcceptedResponseStatus)
	assertNoLegacyPacket(t, backend.writes)
	legacyResponse(t, h, SuccessfulResponseStatus)
	assertLegacyStatus(t, backend.writes, SuccessfulResponseStatus)
	legacyResponse(t, h, SuccessfulResponseStatus)
	assertNoLegacyPacket(t, backend.writes)
	if h.FirstAppliedPack() != first {
		t.Fatal("duplicate response changed applied pack")
	}
}

func TestLegacyDeclinedPackFlushesOptionalQueue(t *testing.T) {
	player := &legacyTestPlayer{protocol: version.Minecraft_1_20.Protocol, writes: make(chan proto.Packet, 3)}
	firstBackend := &legacyTestBackend{writes: make(chan proto.Packet, 2)}
	secondBackend := &legacyTestBackend{writes: make(chan proto.Packet, 2)}
	h := newLegacy117Handler(player, event.Nop)
	first := &Info{URL: "https://example.invalid/one.zip", Origin: DownstreamServerOrigin}
	optional := &Info{URL: "https://example.invalid/two.zip", Origin: DownstreamServerOrigin}
	required := &Info{URL: "https://example.invalid/three.zip", ShouldForce: true, Origin: DownstreamServerOrigin}
	for _, q := range []struct {
		info    *Info
		backend *legacyTestBackend
	}{{first, firstBackend}, {optional, secondBackend}, {required, secondBackend}} {
		if err := h.QueueResourcePackFromBackend(q.info, q.backend); err != nil {
			t.Fatal(err)
		}
	}
	assertLegacyRequest(t, player.writes, first.URL, false)
	legacyResponse(t, h, DeclinedResponseStatus)
	assertLegacyStatus(t, firstBackend.writes, DeclinedResponseStatus)
	assertLegacyStatus(t, secondBackend.writes, DeclinedResponseStatus)
	assertLegacyRequest(t, player.writes, required.URL, true)
	assertNoLegacyPacket(t, player.writes)
	legacyResponse(t, h, AcceptedResponseStatus)
	assertLegacyStatus(t, secondBackend.writes, AcceptedResponseStatus)
	last := &Info{URL: "https://example.invalid/four.zip", Origin: DownstreamServerOrigin}
	if err := h.QueueResourcePackFromBackend(last, secondBackend); err != nil {
		t.Fatal(err)
	}
	assertNoLegacyPacket(t, player.writes)
	legacyResponse(t, h, SuccessfulResponseStatus)
	assertLegacyStatus(t, secondBackend.writes, SuccessfulResponseStatus)
	assertLegacyRequest(t, player.writes, last.URL, false)
}

func TestLegacyQueueWaitsForResponseWrite(t *testing.T) {
	player := &legacyTestPlayer{protocol: version.Minecraft_1_20.Protocol, writes: make(chan proto.Packet, 3)}
	backend := &legacyTestBackend{writes: make(chan proto.Packet, 1)}
	started := make(chan struct{})
	release := make(chan struct{})
	backend.write = func(p proto.Packet) error {
		close(started)
		<-release
		backend.writes <- p
		return nil
	}
	h := newLegacy117Handler(player, event.Nop)
	first := &Info{URL: "https://example.invalid/one.zip", Origin: DownstreamServerOrigin}
	second := &Info{URL: "https://example.invalid/two.zip"}
	third := &Info{URL: "https://example.invalid/three.zip"}
	if err := h.QueueResourcePackFromBackend(first, backend); err != nil {
		t.Fatal(err)
	}
	if err := h.QueueResourcePack(second); err != nil {
		t.Fatal(err)
	}
	assertLegacyRequest(t, player.writes, first.URL, false)
	done := make(chan error, 1)
	go func() {
		_, err := h.OnResourcePackResponse(&ResponseBundle{Status: SuccessfulResponseStatus})
		done <- err
	}()
	legacyCall(t, func() struct{} { <-started; return struct{}{} })
	if err := legacyCall(t, func() error { return h.QueueResourcePack(third) }); err != nil {
		t.Fatal(err)
	}
	assertNoLegacyPacket(t, player.writes)
	close(release)
	if err := legacyCall(t, func() error { return <-done }); err != nil {
		t.Fatal(err)
	}
	assertLegacyStatus(t, backend.writes, SuccessfulResponseStatus)
	assertLegacyRequest(t, player.writes, second.URL, false)
	legacyResponse(t, h, SuccessfulResponseStatus)
	assertLegacyRequest(t, player.writes, third.URL, false)
}

func TestLegacyQueueDoesNotAdvanceWhileRequestWriteIsRunning(t *testing.T) {
	writes := make(chan proto.Packet, 2)
	release := make(chan struct{})
	player := &legacyTestPlayer{protocol: version.Minecraft_1_20.Protocol}
	player.write = func(p proto.Packet) error {
		writes <- p
		if p.(*packet.ResourcePackRequest).URL == "https://example.invalid/one.zip" {
			<-release
		}
		return nil
	}
	h := newLegacy117Handler(player, event.Nop)
	first := &Info{URL: "https://example.invalid/one.zip"}
	second := &Info{URL: "https://example.invalid/two.zip"}
	done := make(chan error, 1)
	go func() { done <- h.QueueResourcePack(first) }()
	assertLegacyRequest(t, writes, first.URL, false)
	if err := legacyCall(t, func() error { return h.QueueResourcePack(second) }); err != nil {
		t.Fatal(err)
	}
	legacyResponse(t, h, AcceptedResponseStatus)
	legacyResponse(t, h, SuccessfulResponseStatus)
	assertNoLegacyPacket(t, writes)
	close(release)
	if err := legacyCall(t, func() error { return <-done }); err != nil {
		t.Fatal(err)
	}
	assertLegacyRequest(t, writes, second.URL, false)
	assertNoLegacyPacket(t, writes)
}

func TestLegacyConcurrentQueuesSendOnce(t *testing.T) {
	const count = 32
	player := &legacyTestPlayer{protocol: version.Minecraft_1_20.Protocol, writes: make(chan proto.Packet, count)}
	h := newLegacy117Handler(player, event.Nop)
	errs := make(chan error, count)
	var queued sync.WaitGroup
	for i := 0; i < count; i++ {
		queued.Add(1)
		go func(i int) {
			defer queued.Done()
			errs <- h.QueueResourcePack(&Info{URL: fmt.Sprintf("https://example.invalid/%d.zip", i)})
		}(i)
	}
	legacyCall(t, func() struct{} { queued.Wait(); return struct{}{} })
	for i := 0; i < count; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	seen := make(map[string]bool, count)
	for i := 0; i < count; i++ {
		p := legacyPacket(t, player.writes)
		req, ok := p.(*packet.ResourcePackRequest)
		if !ok || seen[req.URL] {
			t.Fatalf("duplicate or unexpected request: %#v", p)
		}
		seen[req.URL] = true
		assertNoLegacyPacket(t, player.writes)
		legacyResponse(t, h, SuccessfulResponseStatus)
	}
	if len(seen) != count {
		t.Fatalf("sent %d distinct packs, want %d", len(seen), count)
	}
	assertNoLegacyPacket(t, player.writes)
}

func TestLegacyFailedWriteDoesNotBlockQueue(t *testing.T) {
	writes := make(chan proto.Packet, 3)
	writeErr := errors.New("write failed")
	player := &legacyTestPlayer{protocol: version.Minecraft_1_20.Protocol}
	player.write = func(p proto.Packet) error {
		if p.(*packet.ResourcePackRequest).URL == "https://example.invalid/fail.zip" {
			return writeErr
		}
		writes <- p
		return nil
	}
	backend := &legacyTestBackend{writes: make(chan proto.Packet, 3)}
	h := newLegacy117Handler(player, event.Nop)
	first := &Info{URL: "https://example.invalid/one.zip", Origin: DownstreamServerOrigin}
	failed := &Info{URL: "https://example.invalid/fail.zip", Origin: DownstreamServerOrigin}
	last := &Info{URL: "https://example.invalid/last.zip", Origin: DownstreamServerOrigin}
	for _, info := range []*Info{first, failed, last} {
		if err := h.QueueResourcePackFromBackend(info, backend); err != nil {
			t.Fatal(err)
		}
	}
	assertLegacyRequest(t, writes, first.URL, false)
	got := legacyCall(t, func() error {
		_, err := h.OnResourcePackResponse(&ResponseBundle{Status: SuccessfulResponseStatus})
		return err
	})
	if !errors.Is(got, writeErr) {
		t.Fatalf("error = %v, want %v", got, writeErr)
	}
	assertLegacyStatus(t, backend.writes, SuccessfulResponseStatus)
	assertLegacyStatus(t, backend.writes, DeclinedResponseStatus)
	assertLegacyRequest(t, writes, last.URL, false)
	legacyResponse(t, h, SuccessfulResponseStatus)
	assertLegacyStatus(t, backend.writes, SuccessfulResponseStatus)
	if err := legacyCall(t, func() error { return h.QueueResourcePackFromBackend(failed, backend) }); !errors.Is(err, writeErr) {
		t.Fatalf("immediate send error = %v, want %v", err, writeErr)
	}
	assertNoLegacyPacket(t, backend.writes)
	if err := h.QueueResourcePack(last); err != nil {
		t.Fatal(err)
	}
	assertLegacyRequest(t, writes, last.URL, false)
}

func TestLegacyResponseDuringFailedWrite(t *testing.T) {
	for _, status := range []packet.ResponseStatus{AcceptedResponseStatus, SuccessfulResponseStatus} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			writeErr := errors.New("write failed")
			player := &legacyTestPlayer{protocol: version.Minecraft_1_20.Protocol}
			backend := &legacyTestBackend{writes: make(chan proto.Packet, 1)}
			h := newLegacy117Handler(player, event.Nop)
			info := &Info{URL: "https://example.invalid/pack.zip", Origin: DownstreamServerOrigin}
			player.write = func(proto.Packet) error {
				if _, err := h.OnResourcePackResponse(&ResponseBundle{Status: status}); err != nil {
					return err
				}
				return writeErr
			}
			err := legacyCall(t, func() error { return h.QueueResourcePackFromBackend(info, backend) })
			if status == AcceptedResponseStatus && !errors.Is(err, writeErr) {
				t.Fatalf("accepted response: error = %v, want %v", err, writeErr)
			}
			if status == SuccessfulResponseStatus && err != nil {
				t.Fatalf("completed response must not produce a second failure: %v", err)
			}
			assertLegacyStatus(t, backend.writes, status)
			if h.FirstPendingPack() != nil {
				t.Fatal("failed write left a pending pack")
			}
			if status == SuccessfulResponseStatus && h.FirstAppliedPack() != info {
				t.Fatal("completed response did not apply the pack")
			}
		})
	}
}

type legacyReentrantEvents struct {
	event.Manager
	onStatus func(*PlayerResourcePackStatusEvent)
}

func (e *legacyReentrantEvents) FireParallel(value event.Event, _ ...event.HandlerFunc) {
	e.onStatus(value.(*PlayerResourcePackStatusEvent))
}

func TestLegacyResponseCallbacksCanReenter(t *testing.T) {
	player := &legacyTestPlayer{protocol: version.Minecraft_1_20.Protocol, writes: make(chan proto.Packet, 2)}
	events := &legacyReentrantEvents{Manager: event.Nop}
	h := newLegacy117Handler(player, events)
	first := &Info{URL: "https://example.invalid/one.zip"}
	second := &Info{URL: "https://example.invalid/two.zip"}
	callbackErr := make(chan error, 1)
	events.onStatus = func(e *PlayerResourcePackStatusEvent) {
		if e.Status() == SuccessfulResponseStatus {
			if h.FirstAppliedPack() != first {
				callbackErr <- errors.New("applied state was not visible to callback")
				return
			}
			callbackErr <- h.QueueResourcePack(second)
		}
	}
	if err := h.QueueResourcePack(first); err != nil {
		t.Fatal(err)
	}
	assertLegacyRequest(t, player.writes, first.URL, false)
	legacyResponse(t, h, SuccessfulResponseStatus)
	if err := legacyCall(t, func() error { return <-callbackErr }); err != nil {
		t.Fatal(err)
	}
	assertLegacyRequest(t, player.writes, second.URL, false)
	assertNoLegacyPacket(t, player.writes)
}
