package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/aws/aws-sdk-go/aws/session"
	log "github.com/sirupsen/logrus"
)

const (
	defaultInterval = "15s"
)

var (
	config struct {
		Interval         time.Duration
		PodSelectors     PodSelectors
		ConfigMap        string
		ASGLifecycleHook string
	}
)

func init() {
	kingpin.Flag("interval", "Interval between checks.").
		Default(defaultInterval).DurationVar(&config.Interval)
	kingpin.Flag("pod-selector", "Pod selector specified by <namespace>:<key>=<value>,+.").
		SetValue(&config.PodSelectors)
	kingpin.Flag("pod-selector-configmap", "Name of configMap with pod selector definition. Must be in the same namespace.").
		StringVar(&config.ConfigMap)
	kingpin.Flag("asg-lifecycle-hook", "Name of ASG lifecycle hook to trigger on node Ready.").
		StringVar(&config.ASGLifecycleHook)
}

func main() {
	kingpin.Parse()

	var hooks []Hook
	if config.ASGLifecycleHook != "" {
		awsSess, err := session.NewSession()
		if err != nil {
			log.Fatalf("Failed to setup Kubernetes client: %v", err)
		}

		hooks = append(hooks, NewASGLifecycleHook(awsSess, config.ASGLifecycleHook))
	}

	controller, err := NewNodeController(config.PodSelectors, config.Interval, config.ConfigMap, hooks)
	if err != nil {
		log.Fatal(err)
	}

	stopChan := make(chan struct{}, 1)
	go handleSigterm(stopChan)

	controller.Run(stopChan)
}

func handleSigterm(stopChan chan struct{}) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
	log.Info("Received Term signal. Terminating...")
	close(stopChan)
}
