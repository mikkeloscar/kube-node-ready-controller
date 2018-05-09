package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
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
		IgnoreNodeLabels []string
	}
)

func init() {
	kingpin.Flag("interval", "Interval between checks.").
		Default(defaultInterval).DurationVar(&config.Interval)
	kingpin.Flag("pod-selector", "Pod selector specified by <namespace>:<key>=<value>,+.").
		SetValue(&config.PodSelectors)
	kingpin.Flag("pod-selector-configmap", "Name of configMap with pod selector definition. Must be in the same namespace.").
		StringVar(&config.ConfigMap)
	kingpin.Flag("ignore-node-label", "Ignore nodes based on label. Format <key>=<value>.").
		StringsVar(&config.IgnoreNodeLabels)
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

	ignoreLabels, err := parseNodeLabels(config.IgnoreNodeLabels)
	if err != nil {
		log.Fatal(err)
	}

	controller, err := NewNodeController(config.PodSelectors, ignoreLabels, config.Interval, config.ConfigMap, hooks)
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

func parseNodeLabels(ignoreNodeLabels []string) (map[string]string, error) {
	labels := make(map[string]string, len(ignoreNodeLabels))
	for _, label := range ignoreNodeLabels {
		split := strings.Split(label, "=")
		if len(split) != 2 {
			return nil, fmt.Errorf("failed to parse label defintion '%s'", label)
		}
		labels[split[0]] = split[1]
	}
	return labels, nil
}
