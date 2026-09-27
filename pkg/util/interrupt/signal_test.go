package interrupt

import (
	"os"
	"strings"
	"syscall"
	"testing"
)

// These tests pin the two halves of Gate's signal contract that operators meet
// when a proxy hangs:
//
//   - SIGQUIT must stay a graceful termination signal. Removing it from
//     terminationSignals would make `kill -QUIT` dump and die ungracefully for
//     every supervisor that stops a Gate run with QUIT, dropping players
//     without the shutdown reason Gate reports today.
//   - the signals that do produce a dump must stay out of that list, or the Go
//     runtime's fatal-signal path (dump every goroutine, then exit) never runs
//     and an operator has no way to get a stack dump out of a hung Gate.
//
// Measured on the released Gate 0.74.20-.24 linux_amd64 binaries: QUIT/INT/TERM
// shut down gracefully with zero goroutine lines and exit 0, while `kill -ABRT`
// prints every goroutine with a full stack (~34 kB, 26 goroutines in the
// observed startup state) and exits 2, with no environment variable needed.

func TestTerminationSignalsKeepSIGQUITGracefulAndTheDumpSignalsFatal(t *testing.T) {
	if !containsSignal(terminationSignals, syscall.SIGQUIT) {
		t.Errorf("SIGQUIT must stay a graceful termination signal: %v", terminationSignals)
	}
	for _, fatal := range []os.Signal{syscall.SIGABRT, syscall.SIGSEGV} {
		if containsSignal(terminationSignals, fatal) {
			t.Errorf("%v must stay out of terminationSignals, it is the goroutine dump route: %v",
				fatal, terminationSignals)
		}
	}
}

func TestGoroutineDumpHintNamesTheDumpCommandForSIGQUIT(t *testing.T) {
	hint := GoroutineDumpHint(syscall.SIGQUIT)
	if !strings.Contains(hint, "kill -ABRT <pid>") {
		t.Errorf("the SIGQUIT hint must name the working dump command, got %q", hint)
	}
	if !strings.Contains(hint, "no goroutine dump") {
		t.Errorf("the SIGQUIT hint must say SIGQUIT prints no dump, got %q", hint)
	}
	if !strings.Contains(hint, "graceful") {
		t.Errorf("the SIGQUIT hint must say SIGQUIT is a graceful shutdown, got %q", hint)
	}
}

func TestGoroutineDumpHintIsSilentForOtherTerminationSignals(t *testing.T) {
	for _, sig := range []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM} {
		if hint := GoroutineDumpHint(sig); hint != "" {
			t.Errorf("GoroutineDumpHint(%v) = %q, want no hint", sig, hint)
		}
	}
}

func containsSignal(list []os.Signal, want os.Signal) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
