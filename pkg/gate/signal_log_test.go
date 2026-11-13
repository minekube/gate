package gate

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/util/interrupt"
)

// Gate's shutdown signal path is the one operator-facing behaviour here: Go's
// runtime prints every goroutine stack on SIGQUIT, but Gate registers SIGQUIT as
// a termination signal, so a `kill -QUIT` on a hung proxy shuts it down and
// prints nothing. The shutdown line has to name the signal that does work, or
// an operator (and the support answer that sent them) is left reading a graceful
// shutdown as if it were a dump.

func TestLogTerminationSignalNamesTheGoroutineDumpCommandForSIGQUIT(t *testing.T) {
	log, logs := testLogger(t)

	logTerminationSignal(log, syscall.SIGQUIT)

	line := logs.String()
	require.Contains(t, line, "Received os signal")
	require.Contains(t, line, "kill -ABRT <pid>",
		"the SIGQUIT shutdown line must name the command that actually dumps goroutines:\n%s", line)
	require.Contains(t, line, interrupt.GoroutineDumpHint(syscall.SIGQUIT),
		"the shutdown line must carry the shared hint text operator docs quote:\n%s", line)
}

func TestLogTerminationSignalKeepsThePlainLineForOtherSignals(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			log, logs := testLogger(t)

			logTerminationSignal(log, sig)

			line := logs.String()
			require.Contains(t, line, "Received os signal")
			require.NotContains(t, line, "kill -ABRT",
				"only SIGQUIT is surprising enough to carry the dump hint:\n%s", line)
		})
	}
}
