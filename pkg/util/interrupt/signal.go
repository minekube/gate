package interrupt

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// from https://github.com/kubernetes/kubernetes/blob/c285e781331a3785a7f436042c65c5641ce8a9e9/pkg/util/interrupt/interrupt.go#L28
var terminationSignals = []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT}

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
