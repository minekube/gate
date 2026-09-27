package interrupt

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// from https://github.com/kubernetes/kubernetes/blob/c285e781331a3785a7f436042c65c5641ce8a9e9/pkg/util/interrupt/interrupt.go#L28
var terminationSignals = []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT}

// goroutineDumpCommand is what an operator has to run to make the Go runtime
// print every goroutine stack: SIGABRT is deliberately absent from
// terminationSignals, so os/signal never installs a handler for it and the
// runtime's fatal-signal path (dump all goroutines, then exit) runs instead.
const goroutineDumpCommand = "kill -ABRT <pid>"

// TerminationContext returns a context that is canceled when a termination signal is received.
func TerminationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, terminationSignals...)
}

// Notify returns a channel receives termination signals from the OS until the context is canceled.
func Notify(ctx context.Context) <-chan os.Signal {
	sig := make(chan os.Signal, len(terminationSignals))
	signal.Notify(sig, terminationSignals...)
	go func() {
		<-ctx.Done()
		signal.Stop(sig)
		close(sig)
	}()
	return sig
}

// GoroutineDumpHint returns an operator-facing hint to log next to a received
// termination signal, or "" when there is nothing surprising to say.
//
// SIGQUIT is the case worth a hint. Go's runtime reacts to SIGQUIT by printing
// every goroutine's stack and exiting, which is why "send the proxy a QUIT to
// get a goroutine dump" is the obvious thing to reach for - but SIGQUIT is in
// terminationSignals, so os/signal consumes it and Gate shuts down gracefully
// with no stacks at all. An operator (or a support answer) chasing a hung proxy
// therefore has to be told which signal still produces a dump.
func GoroutineDumpHint(sig os.Signal) string {
	if sig != syscall.SIGQUIT {
		return ""
	}
	return "SIGQUIT is a graceful shutdown and prints no goroutine dump; for a goroutine dump use " +
		goroutineDumpCommand + " (it ends the Gate run, so capture the console first)"
}
