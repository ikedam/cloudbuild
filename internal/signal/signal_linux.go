package signal

import (
	"os"
	"syscall"
)

var (
	// signals to dump stacktraces and continue execution
	stacktraceSignals = []os.Signal{
		syscall.SIGUSR1,
	}

	// signals to dump stacktraces and quit program
	// applied to signals with _SigKill in https://github.com/golang/go/blob/master/src/runtime/sigtab_linux_generic.go
	stacktraceAndQuitSignals = []os.Signal{
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
	}
)
