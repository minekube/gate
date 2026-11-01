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

// deadlineCloseReporter is implemented by connections that record which
// configured deadline closed them.
type deadlineCloseReporter interface {
	deadlineCloseCause() (DeadlineKind, time.Duration, bool)
}

// DeadlineClose reports whether Gate closed c on one of its own configured
// deadlines, which deadline it was, and the budget it exceeded.
//
// A connection that ended for any other reason - the peer closed it, a frame
// could not be decoded, an ordinary write error - reports false, so callers can
// tell "we ended this on a budget" apart from "something else ended this".
func DeadlineClose(c MinecraftConn) (DeadlineKind, time.Duration, bool) {
	reporter, ok := c.(deadlineCloseReporter)
	if !ok {
		return "", 0, false
	}
	return reporter.deadlineCloseCause()
}

// deadlineCloseCause returns the recorded cause, if any.
func (c *minecraftConn) deadlineCloseCause() (DeadlineKind, time.Duration, bool) {
	if c == nil {
		return "", 0, false
	}
	cause := c.deadlineClose.Load()
	if cause == nil {
		return "", 0, false
	}
	return cause.kind, cause.budget, true
}

// recordDeadlineClose records the first deadline that closed this connection.
// The first one wins: a connection closed on its write deadline must not later
// report a read deadline it never reached.
func (c *minecraftConn) recordDeadlineClose(kind DeadlineKind, budget time.Duration) {
	if budget <= 0 {
		return
	}
	c.deadlineClose.CompareAndSwap(nil, &deadlineClose{kind: kind, budget: budget})
}
