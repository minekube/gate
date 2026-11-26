package netmc

import (
	"context"
	"errors"
	"io"
	"net"

	"go.minekube.com/gate/pkg/edition/java/proto/codec"
)

const (
	terminalIODirectionRead  = "read"
	terminalIODirectionWrite = "write"

	terminalIOClassTransportClosed = "transport_closed"
	terminalIOClassTimeout         = "timeout"
	terminalIOClassFrameTooLarge   = "frame_too_large"
	terminalIOClassDecodeError     = "decode_error"
	terminalIOClassWriteFailure    = "write_failure"
)

// minecraftTerminalIOObserver is an optional adapter boundary for owners of a
// backend transport. Its vocabulary is defined above and the callback carries
// no error, address, packet, or payload data. The built-in TCP path does not
// implement it; Connect's tunnel wrapper may use it to enrich its existing
// once-only terminal summary.
type minecraftTerminalIOObserver interface {
	ObserveMinecraftTerminalIO(direction, class string, observed, limit int)
}

func observeMinecraftTerminalIO(conn net.Conn, direction, class string, observed, limit int) {
	if observer, ok := conn.(minecraftTerminalIOObserver); ok {
		observer.ObserveMinecraftTerminalIO(direction, class, observed, limit)
	}
}

func classifyTerminalReadError(err error) (class string, observed, limit int) {
	var frameErr *codec.FrameTooLargeError
	if errors.As(err, &frameErr) {
		return terminalIOClassFrameTooLarge, frameErr.Length, frameErr.Max
	}
	if terminalIOTimeout(err) {
		return terminalIOClassTimeout, 0, 0
	}
	if terminalIOClosed(err) {
		return terminalIOClassTransportClosed, 0, 0
	}
	return terminalIOClassDecodeError, 0, 0
}

func classifyTerminalWriteError(err error) string {
	if terminalIOTimeout(err) {
		return terminalIOClassTimeout
	}
	if terminalIOClosed(err) {
		return terminalIOClassTransportClosed
	}
	return terminalIOClassWriteFailure
}

func terminalIOTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func terminalIOClosed(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, context.Canceled)
}
