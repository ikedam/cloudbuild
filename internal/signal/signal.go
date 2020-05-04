package signal

import (
	"os"
	"os/signal"
	"runtime/pprof"

	"github.com/ikedam/cloudbuild/log"
)

// WithSignalStacktrace dumps stacktrace when receiving signals during running a function
func WithSignalStacktrace(alwaysDump bool, f func(), cleanup func(os.Signal)) {
	signals := []os.Signal{}
	signals = append(signals, stacktraceSignals...)
	signals = append(signals, stacktraceAndQuitSignals...)
	if len(signals) == 0 {
		log.Debug("No signal handlers are set up")
		return
	}
	signalsToDump := []os.Signal{}
	signalsToDump = append(signalsToDump, stacktraceSignals...)
	if alwaysDump {
		signalsToDump = append(signalsToDump, stacktraceAndQuitSignals...)
	}

	log.WithField("signals", signals).Debug("Setup signal handlers")

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

			log.WithField("signal", s).Info("Received signal...")
			for _, signalToDump := range signalsToDump {
				if s == signalToDump {
					log.Info("Dump stacktrace...")
					pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
				}
			}
			for _, signalToQuit := range stacktraceAndQuitSignals {
				if s == signalToQuit {
					log.Info("Cleanup...")
					cleanup(s)
					// go looks exits with 2 for signals.
					// I'm not sure this is a proper way to cause signal to exit.
					os.Exit(2)
				}
			}
		}
	}()
	f()
}
