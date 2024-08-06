package proxy

import (
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/future"
	"go.minekube.com/gate/pkg/internal/mathutil"
	"sync"
	"sync/atomic"
	"time"
)

// chatQueue is a precisely ordered queue which allows for outside entries into the ordered queue through piggybacking timestamps.
type chatQueue struct {
	internalLock sync.Mutex
	player       *connectedPlayer
	chatState    *ChatState
	head         *future.Future[any]
}

// NewChatQueue instantiates a chatQueue for a specific player.
func newChatQueue(player *connectedPlayer) *chatQueue {
	return &chatQueue{
		player:    player,
		chatState: &ChatState{},
		head:      future.New[any]().Complete(nil),
	}
}

func (cq *chatQueue) queueTask(task func(*ChatState, netmc.MinecraftConn) *future.Future[any]) {
	cq.internalLock.Lock()
	defer cq.internalLock.Unlock()

	smc, ok := cq.player.ensureBackendConnection()
	if !ok {
		return
	}
	cq.head = future.ThenCompose(cq.head, func(a any) *future.Future[any] {
		return task(cq.chatState, smc)
	})
}

// QueuePacket queues a packet sent from the player - all packets must wait until this processes to send their packets.
// This maintains order on the server-level for the client insertions of commands and messages. All entries are locked through an internal lock.
//
// - nextPacket: a function mapping LastSeenMessages state to a CompletableFuture that will provide the next-processed packet. This should include the fixed LastSeenMessages.
// - timestamp: the new Instant timestamp of this packet to update the internal chat state.
// - lastSeenMessages: the new LastSeenMessages last seen messages to update the internal chat state.
func (cq *chatQueue) QueuePacket(nextPacket func(*chat.LastSeenMessages) *future.Future[proto.Packet], timestamp time.Time, lastSeenMessages *chat.LastSeenMessages) {
	cq.queueTask(func(chatState *ChatState, smc netmc.MinecraftConn) *future.Future[any] {
		newLastSeenMessages := chatState.UpdateFromMessage(&timestamp, lastSeenMessages)
		return future.ThenCompose(nextPacket(newLastSeenMessages), func(p proto.Packet) *future.Future[any] {
			return writePacket(p, smc)
		})
	})
}

// QueuePacketWithFunction hijacks the latest sent packet's chat state to provide an in-order packet without polling the physical, or prior packets sent through the stream.
func (cq *chatQueue) QueuePacketWithFunction(packetFunction func(*ChatState) proto.Packet) {
	cq.queueTask(func(chatState *ChatState, smc netmc.MinecraftConn) *future.Future[any] {
		packet := packetFunction(chatState)
		return writePacket(packet, smc)
	})
}

// HandleAcknowledgement handles the acknowledgement of packets.
func (cq *chatQueue) HandleAcknowledgement(offset int) {
	cq.queueTask(func(chatState *ChatState, smc netmc.MinecraftConn) *future.Future[any] {
		ackCountToForward := chatState.AccumulateAckCount(offset)
		if ackCountToForward > 0 {
			return writePacket(&chat.ChatAcknowledgement{Offset: ackCountToForward}, smc)
		}
		return future.New[any]().Complete(nil)
	})
}

func writePacket(packet proto.Packet, smc proto.PacketWriter) *future.Future[any] {
	f := future.New[any]()
	if packet == nil {
		f.Complete(nil)
		return f
	}
	go func() {
		_ = smc.WritePacket(packet)
		f.Complete(nil)
	}()
	return f
}

// ChatState tracks the last Secure Chat state that we received from the client. This is important to always have a valid 'last seen' state that is consistent with future and past updates from the client (which may be signed). This state is used to construct 'spoofed' command packets from the proxy to the server.
//   - If we last forwarded a chat or command packet from the client, we have a known 'last seen' that we can reuse.
//   - If we last forwarded a ChatAcknowledgementPacket, the previous 'last seen' cannot be reused. We cannot predict an up-to-date 'last seen', as we do not know which messages the client actually saw.
//   - Therefore, we need to hold back any acknowledgement packets so that we can continue to reuse the last valid 'last seen' state.
//   - However, there is a limit to the number of messages that can remain unacknowledged on the server.
//   - To address this, we know that if the client has moved its 'last seen' window far enough, we can fill in the gap with dummy 'last seen', and it will never be checked.
//
// Note that this is effectively unused for 1.20.5+ clients, as commands without any signature do not send 'last seen' updates.
type ChatState struct {
	lastTimestamp    atomic.Pointer[time.Time]       // time.Time
	lastSeenMessages atomic.Pointer[mathutil.BitSet] // BitSet
	delayedAckCount  atomic.Int32
}

func (cs *ChatState) LastTimestamp() time.Time {
	t := cs.lastTimestamp.Load()
	if t == nil {
		return time.Time{}
	}
	return *t
}

const (
	lastSeenMessagesWindowSize = 20
	minimumDelayedAckCount     = lastSeenMessagesWindowSize
)

var (
	dummyLastSeenMessages = mathutil.BitSet{}
)

func (cs *ChatState) UpdateFromMessage(timestamp *time.Time, lastSeenMessages *chat.LastSeenMessages) *chat.LastSeenMessages {
	if timestamp != nil {
		cs.lastTimestamp.Store(timestamp)
	}
	if lastSeenMessages != nil {
		// We held back some acknowledged messages, so flush that out now that we have a known 'last seen' state again
		delayedAckCount := cs.delayedAckCount.Swap(0)
		cs.lastSeenMessages.Store(&lastSeenMessages.Acknowledged)
		return &chat.LastSeenMessages{
			Offset:       lastSeenMessages.Offset + int(delayedAckCount),
			Acknowledged: lastSeenMessages.Acknowledged,
		}
	}
	return nil
}

func (cs *ChatState) AccumulateAckCount(ackCount int) int {
	delayedAckCount := cs.delayedAckCount.Add(int32(ackCount))
	ackCountToForward := delayedAckCount - minimumDelayedAckCount
	if ackCountToForward >= lastSeenMessagesWindowSize {
		// Because we only forward acknowledgements above the window size, we don't have to shift the previous 'last seen' state
		cs.lastSeenMessages.Store(&dummyLastSeenMessages)
		cs.delayedAckCount.Store(minimumDelayedAckCount)
		return int(ackCountToForward)
	}
	return 0
}

func (cs *ChatState) CreateLastSeen() chat.LastSeenMessages {
	var lastSeenAck mathutil.BitSet
	if ack := cs.lastSeenMessages.Load(); ack != nil {
		lastSeenAck = *ack
	}
	return chat.LastSeenMessages{
		Offset:       0,
		Acknowledged: lastSeenAck,
	}
}
