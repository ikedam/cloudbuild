package signal

import (
	"os"
	"os/signal"
	"runtime/pprof"

	"github.com/ikedam/cloudbuild/log"
)

// WithSignalStacktrace dumps stacktrace when receiving signals during running a function
func WithSignalStacktrace(alwaysDump bool, f func()) {
	signals := []os.Signal{}
	signals = append(signals, stacktraceSignals...)
	if alwaysDump {
		signals = append(signals, stacktraceAndQuitSignals...)
	}
	if len(signals) == 0 {
		log.Debug("No signal handlers are set up")
		return
	}

	log.WithField("signals", signals).Debug("Setup signal handlers for signal handlers are set up")

	c := make(chan os.Signal, 1)
	signal.Notify(c, signals...)
	defer func() {
		// I'm not sure I have to let goroutine exit cleanly.
		signal.Stop(c)
		close(c)
	}()
	go func() {
		for {
			s, ok := <-c
			if !ok {
				break
			}
			log.WithField("signal", s).Info("Received signal...Dump stacktrace...")
			pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
			for _, signalToQuit := range stacktraceAndQuitSignals {
				if s == signalToQuit {
					// go looks exits with 2 for signals.
					// I'm not sure this is a proper way to cause signal to exit.
					os.Exit(2)
				}
			}
		}
	}()
	f()
}
