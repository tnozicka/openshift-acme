package signals

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"k8s.io/klog"
)

var (
	stopChannel = make(chan struct{})
	once        sync.Once

	shutdownSignals = []os.Signal{syscall.SIGINT, syscall.SIGABRT, syscall.SIGTERM}
)

func setupStopChannel() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		s := <-c
		klog.Infof("Received shutdown signal: %s; shutting down...", s)
		close(stopChannel)
		<-c
		klog.Infof("Received second shutdown signal: %s; exiting...", s)
		os.Exit(1) // second signal. Exit directly.
	}()
}

func StopChannel() (stopCh <-chan struct{}) {
	once.Do(setupStopChannel)
	return stopChannel
}
