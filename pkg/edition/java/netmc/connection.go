package netmc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.uber.org/atomic"

	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
)

// MinecraftConn is a Minecraft connection of a client or server.
// The connection is unusable after Close was called and must be recreated.
type MinecraftConn interface {
	// Context returns the context of the connection.
	// This Context is canceled on Close and can be used to attach more context values to a connection.
	Context() context.Context
	// Close closes the connection, if not already, and calls SessionHandler.Disconnected.
	// It is okay to call this method multiple times.
	// If the connection is in a closing state Close blocks until the connection completed the close.
	// To check whether a connection is closed use Closed.
	Close() error

	// State returns the current state of the connection.
	State() *state.Registry
	// Protocol returns the protocol version of the connection.
	Protocol() proto.Protocol
	// RemoteAddr returns the remote address of the connection.
	RemoteAddr() net.Addr
	// LocalAddr returns the local address of the connection.
	LocalAddr() net.Addr
	// Type returns the connection type of the connection.
	Type() phase.ConnectionType
	// SessionHandler returns the session handler of the connection.
	SessionHandler() SessionHandler

	// SetSessionHandler sets the session handler for this connection
	// and calls Deactivated() on the old handler and Activated() on the new handler.
	SetSessionHandler(SessionHandler)
	// SetType sets the connection type of the connection.
	SetType(phase.ConnectionType)

	StateChanger
	PacketWriter
}

// Closed returns true if the connection is closed.
func Closed(c interface{ Context() context.Context }) bool {
	return c.Context().Err() != nil
}

// PacketWriter is the interface for writing packets to the underlying connection.
type PacketWriter interface {
	// WritePacket writes a packet to the connection's
	// write buffer and flushes the complete buffer afterwards.
	//
	// The connection will be closed on any error encountered!
	WritePacket(p proto.Packet) (err error)
	// Write encodes and writes payload to the connection's
	// write buffer and flushes the complete buffer afterwards.
	Write(payload []byte) (err error)

	// BufferPacket writes a packet into the connection's write buffer.
	BufferPacket(packet proto.Packet) (err error)
	// BufferPayload writes payload (containing packet id + data) to the connection's write buffer.
	BufferPayload(payload []byte) (err error)
	// Flush flushes the buffered data to the connection.
	Flush() error
}

// StateChanger updates state of a connection.
type StateChanger interface {
	// SetProtocol switches the connection's protocol version.
	SetProtocol(proto.Protocol)
	// SetState switches the connection's state.
	SetState(state *state.Registry)
	// SetCompressionThreshold sets the compression threshold of the connection.
	// packet.SetCompression should be sent beforehand.
	SetCompressionThreshold(threshold int) error
	// EnableEncryption takes the secret key negotiated between the client and
	// the server to enable encryption on the connection.
	EnableEncryption(secret []byte) error
}

// SessionHandler handles received packets from the associated connection.
//
// Since connections transition between states packets need to be handled differently,
// this behaviour is divided between sessions by session handlers.
type SessionHandler interface {
	HandlePacket(pc *proto.PacketContext) // Called to handle incoming known or unknown packet.
	Disconnected()                        // Called when connection is closing, to teardown the session.

	Activated()   // Called when the connection is now managed by this SessionHandler.
	Deactivated() // Called when the connection is no longer managed by this SessionHandler.
}

// NewMinecraftConn returns a new MinecraftConn and the func to start the blocking read-loop.
func NewMinecraftConn(
	ctx context.Context,
	base net.Conn,
	direction proto.Direction,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	compressionLevel int,
) (conn MinecraftConn, startReadLoop func()) {
	in := proto.ServerBound  // reads from client are server bound (proxy <- client)
	out := proto.ClientBound // writes to client are client bound (proxy -> client)
	logName := "client"
	if direction == proto.ClientBound { // if is a backend server connection
		in = proto.ClientBound  // reads from backend are client bound (proxy <- backend)
		out = proto.ServerBound // writes to backend are server bound (proxy -> backend)
		logName = "server"
	}

	log := logr.FromContextOrDiscard(ctx).WithName(logName)
	ctx = logr.NewContext(ctx, log)

	ctx, cancel := context.WithCancel(ctx)
	c := &minecraftConn{
		log:       log,
		c:         base,
		ctx:       ctx,
		cancelCtx: cancel,
		rd:        NewReader(base, in, readTimeout, log),
		wr:        NewWriter(base, out, writeTimeout, compressionLevel, log),
		state:     state.Handshake,
		protocol:  version.Minecraft_1_7_2.Protocol,
		connType:  phase.Undetermined,
	}
	return c, c.startReadLoop
}

// minecraftConn is a Minecraft connection.
// It may be the connection of client -> proxy or proxy -> backend server.
type minecraftConn struct {
	c   net.Conn    // underlying connection
	log logr.Logger // connections own logger

	rd Reader
	wr Writer

	ctx             context.Context // is canceled when connection closed
	cancelCtx       context.CancelFunc
	closeOnce       sync.Once   // Makes sure the connection is closed once, while blocking proceeding calls.
	knownDisconnect atomic.Bool // Silences disconnect (any error is known)

	protocol proto.Protocol // Client's protocol version.

	mu       sync.RWMutex         // Protects following fields
	state    *state.Registry      // Client state.
	connType phase.ConnectionType // Connection type

	sessionHandlerMu struct {
		sync.RWMutex
		SessionHandler // The current session handler.
	}
}

// StartReadLoop is the main goroutine of this connection and
// reads packets to pass them further to the current SessionHandler.
// Close will be called on method return.
func (c *minecraftConn) startReadLoop() {
	// Make sure to close connection on return, if not already closed
	defer func() { _ = c.closeKnown(false) }()

	next := func() bool {
		// Read next packet from underlying connection.
		packetCtx, err := c.rd.ReadPacket()
		if err != nil {
			if errors.Is(err, ErrReadPacketRetry) {
				// Sleep briefly and try again
				time.Sleep(time.Millisecond * 5)
				return true
			}
			return false
		}

		// TODO wrap packetCtx into struct with source info
		// (minecraftConn) and chain into packet interceptor to...
		//  - packet interception
		//  - statistics / count bytes
		//  - in turn call session handler

		// Handle packet by connection's session handler.
		c.SessionHandler().HandlePacket(packetCtx)
		return true
	}

	// Using two for loops to optimize for calling "defer, recover" less often
	// and be able to continue the loop in case of panic.

	cond := func() bool { return !Closed(c) && next() }
	loop := func() (ok bool) {
		defer func() { // Catch any panics
			if r := recover(); r != nil {
				c.log.Error(nil, "recovered panic in packets read loop", "panic", r)
				ok = true // recovered, keep going
			}
		}()
		for cond() {
		}
		return false
	}

	for loop() {
	}
}

func (c *minecraftConn) Context() context.Context { return c.ctx }

func (c *minecraftConn) Flush() error {
	err := c.wr.Flush()
	if err != nil {
		c.closeOnErr(err)
	}
	return err
}

func (c *minecraftConn) WritePacket(p proto.Packet) (err error) {
	if Closed(c) {
		return ErrClosedConn
	}
	defer func() { c.closeOnErr(err) }()
	if err = c.BufferPacket(p); err != nil {
		return err
	}
	return c.Flush()
}

func (c *minecraftConn) Write(payload []byte) (err error) {
	if Closed(c) {
		return ErrClosedConn
	}
	defer func() { c.closeOnErr(err) }()
	if _, err = c.wr.Write(payload); err != nil {
		return err
	}
	return c.Flush()
}

func (c *minecraftConn) BufferPacket(packet proto.Packet) (err error) {
	if Closed(c) {
		return ErrClosedConn
	}
	defer func() { c.closeOnErr(err) }()
	_, err = c.wr.WritePacket(packet)
	return err
}

func (c *minecraftConn) BufferPayload(payload []byte) (err error) {
	if Closed(c) {
		return ErrClosedConn
	}
	defer func() { c.closeOnErr(err) }()
	_, err = c.wr.Write(payload)
	return err
}

func (c *minecraftConn) closeOnErr(err error) {
	if err == nil {
		return
	}
	_ = c.Close()
	if err == ErrClosedConn {
		return // Don't log this error
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && errs.IsConnClosedErr(opErr.Err) {
		return // Don't log this error
	}
	c.log.V(1).Info("error writing packet, closing connection", "err", err)
}

func (c *minecraftConn) Close() error {
	return c.closeKnown(true)
}

// ErrClosedConn indicates a connection is already closed.
var ErrClosedConn = errors.New("connection is closed")

func (c *minecraftConn) closeKnown(markKnown bool) (err error) {
	alreadyClosed := true
	c.closeOnce.Do(func() {
		alreadyClosed = false
		if markKnown {
			c.knownDisconnect.Store(true)
		}

		c.cancelCtx()
		err = c.c.Close()

		if sh := c.SessionHandler(); sh != nil {
			sh.Disconnected()

			if p, ok := sh.(interface{ PlayerLog() logr.Logger }); ok && !c.knownDisconnect.Load() {
				p.PlayerLog().Info("player has disconnected", "sessionHandler", fmt.Sprintf("%T", sh))
			}
		}
	})
	if alreadyClosed {
		err = ErrClosedConn
	}
	return err
}

// CloseWith closes the connection after writing the packet.
func CloseWith(c MinecraftConn, packet proto.Packet) (err error) {
	if Closed(c) {
		return ErrClosedConn
	}
	defer func() {
		err = c.Close()
	}()

	//c.mu.Lock()
	//p := c.protocol
	//s := c.state
	//c.mu.Unlock()

	//is18 := p.GreaterEqual(proto.Minecraft_1_8)
	//isLegacyPing := s == state.Handshake || s == state.Status
	//if is18 || isLegacyPing {
	if mc, ok := c.(*minecraftConn); ok {
		mc.knownDisconnect.Store(true)
	}
	_ = c.WritePacket(packet)
	//} else {
	// ??? 1.7.x versions have a race condition with switching protocol versions,
	// so just explicitly close the connection after a short while.
	// c.setAutoReading(false)
	//go func() {
	//	time.Sleep(time.Millisecond * 250)
	//	c.knownDisconnect.Store(true)
	//	_ = c.WritePacket(packet)
	//}()
	//}
	return
}

// KnownDisconnect returns true if the connection was or will be expectedly closed by the server.
func KnownDisconnect(c MinecraftConn) bool {
	if mc, ok := c.(*minecraftConn); ok {
		return mc.knownDisconnect.Load()
	}
	return false
}

// CloseUnknown closes the connection on for an unexpected disconnect.
// Use MinecraftConn.Close to prevent logging of disconnects that are expected.
func CloseUnknown(c MinecraftConn) error {
	if mc, ok := c.(*minecraftConn); ok {
		return mc.closeKnown(false)
	}
	return c.Close()
}

func (c *minecraftConn) RemoteAddr() net.Addr {
	return c.c.RemoteAddr()
}

func (c *minecraftConn) LocalAddr() net.Addr {
	return c.c.LocalAddr()
}

func (c *minecraftConn) Protocol() proto.Protocol {
	return c.protocol
}

func (c *minecraftConn) SetProtocol(protocol proto.Protocol) {
	c.protocol = protocol
	c.rd.SetProtocol(protocol)
	c.wr.SetProtocol(protocol)
	// TODO remove minecraft de/encoder when legacy handshake handling
}

func (c *minecraftConn) State() *state.Registry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *minecraftConn) SetState(state *state.Registry) {
	c.mu.Lock()
	c.state = state
	c.rd.SetState(state)
	c.wr.SetState(state)
	c.mu.Unlock()
}

func (c *minecraftConn) Type() phase.ConnectionType {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connType
}

func (c *minecraftConn) SetType(connType phase.ConnectionType) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connType = connType
}

func (c *minecraftConn) SessionHandler() SessionHandler {
	c.sessionHandlerMu.RLock()
	defer c.sessionHandlerMu.RUnlock()
	return c.sessionHandlerMu.SessionHandler
}

func (c *minecraftConn) SetSessionHandler(handler SessionHandler) {
	c.sessionHandlerMu.Lock()
	defer c.sessionHandlerMu.Unlock()
	if c.sessionHandlerMu.SessionHandler != nil {
		c.sessionHandlerMu.SessionHandler.Deactivated()
	}
	c.sessionHandlerMu.SessionHandler = handler
	handler.Activated()
}

// SetCompressionThreshold sets the compression threshold on the connection.
// You are responsible for sending packet.SetCompression beforehand.
func (c *minecraftConn) SetCompressionThreshold(threshold int) error {
	c.log.V(1).Info("update compression", "threshold", threshold)
	err := c.rd.SetCompressionThreshold(threshold)
	if err != nil {
		return err
	}
	return c.wr.SetCompressionThreshold(threshold)
}

func (c *minecraftConn) EnableEncryption(secret []byte) error {
	err := c.rd.EnableEncryption(secret)
	if err != nil {
		return err
	}
	return c.wr.EnableEncryption(secret)
}

// Conn exports the hidden underlying connection and can be retrieved with interface assertion.
func (c *minecraftConn) Conn() net.Conn {
	return c.c
}

// Assert is a utility func that asserts a connection implements an interface T.
//
// e.g. usage `Assert[GameProfileProvider](connection)`
func Assert[T any](c any) (T, bool) {
	i, ok := c.(T)
	if ok {
		return i, true
	}
	// Conn is a hidden method used to export the underlying connection.
	// Also need to check if underlying implements T.
	underlying, ok := c.(interface{ Conn() net.Conn })
	if !ok {
		var t T
		return t, false
	}
	i, ok = underlying.(T)
	return i, ok
}

// SendKeepAlive sends a keep-alive packet to the connection if in Play state.
// This prevents a connection timeout.
func SendKeepAlive(c interface {
	State() *state.Registry
	WritePacket(proto.Packet) error
}) error {
	if c.State() == state.Play {
		return c.WritePacket(&packet.KeepAlive{
			RandomID: int64(randomUint64()),
		})
	}
	return nil
}
func randomUint64() uint64 {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf) // Always succeeds, no need to check error
	return binary.LittleEndian.Uint64(buf)
}
