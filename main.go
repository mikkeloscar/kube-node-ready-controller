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
		Interval     time.Duration
		PodSelectors PodSelectors
		ConfigMap    string
	}
)

func init() {
	kingpin.Flag("interval", "Interval between checks.").
		Default(defaultInterval).DurationVar(&config.Interval)
	kingpin.Flag("pod-selector", "Pod selector specified by <namespace>:<key>=<value>,+.").
		SetValue(&config.PodSelectors)
	kingpin.Flag("pod-selector-configmap", "Name of configMap with pod selector definition. Must be in the same namespace.").
		StringVar(&config.ConfigMap)
}

func main() {
	kingpin.Parse()

	controller, err := NewNodeController(config.PodSelectors, config.Interval, config.ConfigMap)
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
	log.Info("Received Term signal. Terminating...")
	close(stopChan)
}
