package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"go.uber.org/atomic"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/codec"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
)

// sessionHandler handles received packets from the associated connection.
//
// Since connections transition between states packets need to be handled differently,
// this behaviour is divided between sessions by sessionHandlers.
type sessionHandler interface {
	handlePacket(pc *proto.PacketContext) // Called to handle incoming known or unknown packet.
	disconnected()                        // Called when connection is closing, to teardown the session.

	activated()   // Called when the connection is now managed by this sessionHandler.
	deactivated() // Called when the connection is no longer managed by this sessionHandler.
}

// minecraftConn is a Minecraft connection from the
// client -> proxy or proxy -> server (backend).
// readLoop owns these fields
type minecraftConn struct {
	proxy *Proxy      // convenient backreference
	log   logr.Logger // connections own logger
	c     net.Conn    // underlying connection

	readBuf *bufio.Reader
	decoder *codec.Decoder

	writeBuf *bufio.Writer
	encoder  *codec.Encoder

	closed          chan struct{} // indicates connection is closed
	closeOnce       sync.Once     // Makes sure the connection is closed once, while blocking proceeding calls.
	knownDisconnect atomic.Bool   // Silences disconnect (any error is known)

	protocol proto.Protocol // Client's protocol version.

	mu             sync.RWMutex    // Protects following fields
	state          *state.Registry // Client state.
	connType       connectionType  // Connection type
	sessionHandler sessionHandler  // The current session handler.
}

// newMinecraftConn returns a new Minecraft client connection.
func newMinecraftConn(
	base net.Conn,
	proxy *Proxy,
	playerConn bool,
) (conn *minecraftConn) {
	in := proto.ServerBound  // reads from client are server bound (proxy <- client)
	out := proto.ClientBound // writes to client are client bound (proxy -> client)
	logName := "client"
	if !playerConn { // if a backend server connection
		in = proto.ClientBound  // reads from backend are client bound (proxy <- backend)
		out = proto.ServerBound // writes to backend are server bound (proxy -> backend)
		logName = "server"
	}

	log := proxy.log.WithName(logName)
	writeBuf := bufio.NewWriter(base)
	readBuf := bufio.NewReader(base)

	return &minecraftConn{
		proxy:    proxy,
		log:      log,
		c:        base,
		closed:   make(chan struct{}),
		writeBuf: writeBuf,
		readBuf:  readBuf,
		encoder:  codec.NewEncoder(writeBuf, out, log.V(2).WithName("encoder")),
		decoder:  codec.NewDecoder(readBuf, in, log.V(2).WithName("decoder")),
		state:    state.Handshake,
		protocol: version.Minecraft_1_7_2.Protocol,
		connType: undeterminedConnectionType,
	}
}

// readLoop is the main goroutine of this connection and
// reads packets to pass them further to the current sessionHandler.
// close will be called on method return.
func (c *minecraftConn) readLoop() {
	// Make sure to close connection on return, if not already closed
	defer func() { _ = c.closeKnown(false) }()

	readTimeout := time.Duration(c.config().ReadTimeout) * time.Millisecond

	next := func() bool {
		// Set read timeout to wait for client to send a packet
		_ = c.c.SetReadDeadline(time.Now().Add(readTimeout))

		// Read next packet from underlying connection.
		packetCtx, err := c.decoder.Decode()
		if err != nil && !errors.Is(err, proto.ErrDecoderLeftBytes) { // Ignore this error.
			if c.handleReadErr(err) {
				c.log.V(1).Info("Error reading packet, recovered", "err", err)
				// Sleep briefly and try again
				time.Sleep(time.Millisecond * 5)
				return true
			}
			c.log.V(1).Info("Error reading packet, closing connection", "err", err)
			return false
		}

		// TODO wrap packetCtx into struct with source info
		// (minecraftConn) and chain into packet interceptor to...
		//  - packet interception
		//  - statistics / count bytes
		//  - in turn call session handler

		// Handle packet by connection's session handler.
		c.SessionHandler().handlePacket(packetCtx)
		return true
	}

	cond := func() bool { return !c.Closed() && next() }
	loop := func() (ok bool) {
		defer func() { // Catch any panics
			if r := recover(); r != nil {
				c.log.Error(nil, "Recovered panic in packets read loop", "panic", r)
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

// handles error when read the next packet
func (c *minecraftConn) handleReadErr(err error) (recoverable bool) {
	var silentErr *errs.SilentError
	if errors.As(err, &silentErr) {
		c.log.V(1).Info("silentErr: error reading next packet, unrecoverable and closing connection", "err", err)
		return false
	}
	// Immediately retry for EAGAIN
	if errors.Is(err, syscall.EAGAIN) {
		return true
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		if netErr.Temporary() {
			// Immediately retry for temporary network errors
			return true
		} else if netErr.Timeout() {
			// Read timeout, disconnect
			c.log.Error(err, "read timeout")
			return false
		} else if errs.IsConnClosedErr(netErr.Err) {
			// Connection is already closed
			return false
		}
	}
	// Immediately break for known unrecoverable errors
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.Canceled) ||
		errors.Is(err, io.ErrNoProgress) || errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, io.ErrShortBuffer) || errors.Is(err, syscall.EBADF) ||
		strings.Contains(err.Error(), "use of closed file") {
		return false
	}
	c.log.Error(err, "error reading next packet, unrecoverable and closing connection")
	return false
}

// Flush writes the buffered data to connection.
func (c *minecraftConn) flush() (err error) {
	defer func() { c.closeOnErr(err) }()
	deadline := time.Now().Add(time.Millisecond * time.Duration(c.config().ConnectionTimeout))
	if err = c.c.SetWriteDeadline(deadline); err != nil {
		// Handle err in case the connection is
		// already closed and can't write to.
		return err
	}
	// Must flush in sync with encoder, or we may get an
	// io.ErrShortWrite when flushing while encoder is already writing.
	return c.encoder.Sync(c.writeBuf.Flush)
}

func (c *minecraftConn) closeOnErr(err error) {
	if err == nil {
		return
	}
	_ = c.close()
	if err == ErrClosedConn {
		return // Don't log this error
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && errs.IsConnClosedErr(opErr.Err) {
		return // Don't log this error
	}
	c.log.V(1).Info("error writing packet, closing connection", "err", err)
}

// WritePacket writes a packet to the connection's
// write buffer and flushes the complete buffer afterwards.
//
// The connection will be closed on any error encountered!
func (c *minecraftConn) WritePacket(p proto.Packet) (err error) {
	if c.Closed() {
		return ErrClosedConn
	}
	defer func() { c.closeOnErr(err) }()
	if err = c.BufferPacket(p); err != nil {
		return err
	}
	return c.flush()
}

// Write encodes and writes payload to the connection's
// write buffer and flushes the complete buffer afterwards.
func (c *minecraftConn) Write(payload []byte) (err error) {
	if c.Closed() {
		return ErrClosedConn
	}
	defer func() { c.closeOnErr(err) }()
	if _, err = c.encoder.Write(payload); err != nil {
		return err
	}
	return c.flush()
}

// BufferPacket writes a packet into the connection's write buffer.
func (c *minecraftConn) BufferPacket(packet proto.Packet) (err error) {
	if c.Closed() {
		return ErrClosedConn
	}
	defer func() { c.closeOnErr(err) }()
	_, err = c.encoder.WritePacket(packet)
	return err
}

// BufferPayload writes payload (containing packet id + data) to the connection's write buffer.
func (c *minecraftConn) BufferPayload(payload []byte) (err error) {
	if c.Closed() {
		return ErrClosedConn
	}
	defer func() { c.closeOnErr(err) }()
	_, err = c.encoder.Write(payload)
	return err
}

// returns the proxy's config
func (c *minecraftConn) config() *config.Config {
	return c.proxy.config
}

// close closes the connection, if not already,
// and calls disconnected() on the current sessionHandler.
// It is okay to call this method multiple times as it will only
// run once but blocks if currently closing already.
func (c *minecraftConn) close() error {
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

		close(c.closed)
		err = c.c.Close()

		if sh := c.SessionHandler(); sh != nil {
			sh.disconnected()

			if p, ok := sh.(interface{ player_() *connectedPlayer }); ok && !c.knownDisconnect.Load() {
				p.player_().log.Info("player has disconnected",
					"sessionHandler", fmt.Sprintf("%T", sh))
			}
		}
	})
	if alreadyClosed {
		err = ErrClosedConn
	}
	return err
}

// Closes the connection after writing the packet.
func (c *minecraftConn) closeWith(packet proto.Packet) (err error) {
	if c.Closed() {
		return ErrClosedConn
	}
	defer func() {
		err = c.close()
	}()

	//c.mu.Lock()
	//p := c.protocol
	//s := c.state
	//c.mu.Unlock()

	//is18 := p.GreaterEqual(proto.Minecraft_1_8)
	//isLegacyPing := s == state.Handshake || s == state.Status
	//if is18 || isLegacyPing {
	c.knownDisconnect.Store(true)
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

// Closed returns true if the connection is closed.
func (c *minecraftConn) Closed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func (c *minecraftConn) RemoteAddr() net.Addr {
	return c.c.RemoteAddr()
}

func (c *minecraftConn) Protocol() proto.Protocol {
	return c.protocol
}

// setProtocol sets the connection's protocol version.
func (c *minecraftConn) setProtocol(protocol proto.Protocol) {
	c.protocol = protocol
	c.decoder.SetProtocol(protocol)
	c.encoder.SetProtocol(protocol)
	// TODO remove minecraft de/encoder when legacy handshake handling
}

func (c *minecraftConn) State() *state.Registry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *minecraftConn) setState(state *state.Registry) {
	c.mu.Lock()
	c.state = state
	c.decoder.SetState(state)
	c.encoder.SetState(state)
	c.mu.Unlock()
}

func (c *minecraftConn) Type() connectionType {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connType
}

func (c *minecraftConn) setType(connType connectionType) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connType = connType
}

func (c *minecraftConn) SessionHandler() sessionHandler {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionHandler
}

// setSessionHandler sets the session handle for this connection
// and calls deactivated() on the old and activated() on the new.
func (c *minecraftConn) setSessionHandler(handler sessionHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setSessionHandler0(handler)
}

// same as setSessionHandler but without mutex locking
func (c *minecraftConn) setSessionHandler0(handler sessionHandler) {
	if c.sessionHandler != nil {
		c.sessionHandler.deactivated()
	}
	c.sessionHandler = handler
	handler.activated()
}

// SetCompressionThreshold sets the compression threshold on the connection.
// You are responsible for sending packet.SetCompression beforehand.
func (c *minecraftConn) SetCompressionThreshold(threshold int) error {
	c.log.V(1).Info("update compression", "threshold", threshold)
	c.decoder.SetCompressionThreshold(threshold)
	return c.encoder.SetCompression(threshold, c.config().Compression.Level)
}

// SendKeepAlive sends a keep-alive packet to the connection if in Play state.
func (c *minecraftConn) SendKeepAlive() error {
	if c.State() == state.Play {
		return c.WritePacket(&packet.KeepAlive{RandomID: int64(randomUint64())})
	}
	return nil
}

// takes the secret key negotiated between the client and the
// server to enable encryption on the connection.
func (c *minecraftConn) enableEncryption(secret []byte) error {
	decryptReader, err := codec.NewDecryptReader(c.readBuf, secret)
	if err != nil {
		return err
	}
	encryptWriter, err := codec.NewEncryptWriter(c.writeBuf, secret)
	if err != nil {
		return err
	}
	c.decoder.SetReader(decryptReader)
	c.encoder.SetWriter(encryptWriter)
	return nil
}

// Inbound is an incoming connection to the proxy.
type Inbound interface {
	Protocol() proto.Protocol // The current protocol version the connection uses.
	VirtualHost() net.Addr    // The hostname, the client sent us, to join the server, if applicable.
	RemoteAddr() net.Addr     // The player's IP address.
	Active() bool             // Whether the connection remains active.
	// Closed returns a receive-only channel that can be used to know when the connection was closed.
	// (e.g. for canceling work in an event handler)
	Closed() <-chan struct{}
}
