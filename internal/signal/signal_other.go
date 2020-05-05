// +build !linux

package signal

import "os"

var (
	// signals to dump stacktraces and continue execution
	stacktraceSignals = []os.Signal{}

	// signals to dump stacktraces and quit program
	stacktraceAndQuitSignals = []os.Signal{}
)
