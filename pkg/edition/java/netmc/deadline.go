package netmc

import "time"

// DeadlineKind names a configured budget Gate closes a connection on.
type DeadlineKind string

const (
	// DeadlineWrite is the per-flush write budget, config writeTimeout. It is
	// exceeded when a flush cannot drain because the peer stopped reading.
	DeadlineWrite DeadlineKind = "writeTimeout"
	// DeadlineRead is the idle read budget, config readTimeout. It is exceeded
	// when the peer stops sending anything at all.
	DeadlineRead DeadlineKind = "readTimeout"
)

// deadlineClose is the recorded cause of a close Gate itself caused.
type deadlineClose struct {
	kind   DeadlineKind
	budget time.Duration
}

// DeadlineCloseRecorder is implemented by a connection that can record which of
// Gate's configured deadlines closed it and report that afterwards.
//
// These methods are exported, and deliberately not part of MinecraftConn, for
// one reason: a connection wrapper embeds the MinecraftConn *interface*, and an
// embedded interface promotes only the methods declared on it. A wrapper that
// must stay reportable therefore forwards them explicitly - Gate's
// connectedPlayer (the client connection of every player) forwards all three.
// Without forwarding, a close Gate performed on its own budget reads exactly
// like a peer disconnect, which is the ambiguity DeadlineClose exists to remove.
type DeadlineCloseRecorder interface {
	// RecordDeadlineClose records the first configured deadline that closed
	// this connection. Later calls are ignored, and a non-positive budget is
	// ignored because it names no deadline.
	RecordDeadlineClose(kind DeadlineKind, budget time.Duration)
	// DeadlineCloseCause reports the recorded deadline, if any.
	DeadlineCloseCause() (DeadlineKind, time.Duration, bool)
	// FarewellBudgetExceeded records, and logs once, that the connection's
	// farewell write spent its budget, which is what closed it.
	FarewellBudgetExceeded(budget time.Duration)
}

// DeadlineClose reports whether Gate closed c on one of its own configured
// deadlines, which deadline it was, and the budget it exceeded.
//
// A connection that ended for any other reason - the peer closed it, a frame
// could not be decoded, an ordinary write error - reports false, so callers can
// tell "we ended this on a budget" apart from "something else ended this".
// A wrapper that does not forward DeadlineCloseCause also reports false: see
// DeadlineCloseRecorder.
func DeadlineClose(c MinecraftConn) (DeadlineKind, time.Duration, bool) {
	reporter, ok := c.(interface {
		DeadlineCloseCause() (DeadlineKind, time.Duration, bool)
	})
	if !ok {
		return "", 0, false
	}
	return reporter.DeadlineCloseCause()
}

// DeadlineCloseCause returns the recorded cause, if any.
func (c *minecraftConn) DeadlineCloseCause() (DeadlineKind, time.Duration, bool) {
	if c == nil {
		return "", 0, false
	}
	cause := c.deadlineClose.Load()
	if cause == nil {
		return "", 0, false
	}
	return cause.kind, cause.budget, true
}

// RecordDeadlineClose records the first deadline that closed this connection.
// The first one wins: a connection closed on its write deadline must not later
// report a read deadline it never reached.
func (c *minecraftConn) RecordDeadlineClose(kind DeadlineKind, budget time.Duration) {
	if budget <= 0 {
		return
	}
	c.deadlineClose.CompareAndSwap(nil, &deadlineClose{kind: kind, budget: budget})
}
