package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/alecthomas/kingpin.v2"

	log "github.com/Sirupsen/logrus"
)

const (
	defaultInterval = "15s"
)

var (
	config struct {
		Interval       time.Duration
		PodIdentifiers PodIdentifiers
	}
)

func init() {
	kingpin.Flag("interval", "Interval between checks.").
		Default(defaultInterval).DurationVar(&config.Interval)
	kingpin.Flag("pod-identifier", "Pod identifier specified by <namespace>:<key>=<value>,+.").
		SetValue(&config.PodIdentifiers)
}

func main() {
	kingpin.Parse()

	controller, err := NewNodeController(config.PodIdentifiers, config.Interval)
	if err != nil {
		log.Fatal(err)
	}

	stopChan := make(chan struct{}, 1)
	go handleSigterm(stopChan)

	controller.Run(stopChan)
}

func handleSigterm(stopChan chan struct{}) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-signals
	log.Infoln("received Term signal. Terminating...")
	close(stopChan)
}
