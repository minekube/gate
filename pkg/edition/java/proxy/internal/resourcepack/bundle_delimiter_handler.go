package resourcepack

import (
	"errors"
	"fmt"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/future"
	"sync"
)

type BundleDelimiterHandler struct {
	Player interface {
		proto.PacketWriter
		State() states.State
		BackendState() *states.State
	}

	mu                          sync.RWMutex
	finishedBundleSessionFuture *Future
	inBundleSession             bool
}

type Future = future.Chan[error]

// InBundleSession returns true if the player is in the process of receiving multiple packets.
func (b *BundleDelimiterHandler) InBundleSession() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.inBundleSession
}

// ToggleBundleSession toggles the player to be in the process of receiving multiple packets
// from the backend server via a packet bundle.
func (b *BundleDelimiterHandler) ToggleBundleSession() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.inBundleSession {
		b.finishedBundleSessionFuture.Complete(nil)
		b.finishedBundleSessionFuture = nil
	} else {
		b.finishedBundleSessionFuture = future.NewChan[error]()
	}
	b.inBundleSession = !b.inBundleSession
}

// BundlePackets bundles all packets sent in the given function.
func (b *BundleDelimiterHandler) BundlePackets(sendPackets Runnable) *future.Chan[error] {
	backend := b.Player.BackendState()
	if backend == nil {
		return future.Completed(sendPackets())
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	inBundleSession := b.inBundleSession
	finishedBundleSessionFuture := b.finishedBundleSessionFuture

	f := future.NewChan[error]()
	if inBundleSession {
		finishedBundleSessionFuture.ThenAccept(func(err error) {
			f.Complete(errors.Join(err, sendPackets()))
		})
	} else {
		var err error
		if *backend == states.PlayState {
			err = b.sendPackets(sendPackets)
		} else {
			err = sendPackets()
		}
		f.Complete(err)
	}
	return f
}

type Runnable func() error

func (b *BundleDelimiterHandler) sendPackets(sendPackets Runnable) (err error) {
	if err = b.writeBundleDelimiter(); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, b.writeBundleDelimiter()) }()
	return sendPackets()
}

var delim = &packet.BundleDelimiter{}

func (b *BundleDelimiterHandler) writeBundleDelimiter() error {
	err := b.Player.WritePacket(delim)
	if err != nil {
		return fmt.Errorf("error writing bundle delimiter: %w", err)
	}
	return nil
}
