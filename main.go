package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	pkgAWS "github.com/mikkeloscar/kube-node-ready-controller/pkg/aws"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

const (
	defaultInterval       = "15s"
	defaultMetricsAddress = ":7979"
)

var (
	config struct {
		Interval                 time.Duration
		MetricsAddress           string
		PodSelectors             PodSelectors
		ConfigMap                string
		ASGLifecycleHook         string
		IgnoreNodeLabels         []string
		EnableNodeStartUpMetrics bool
	}
)

func init() {
	kingpin.Flag("interval", "Interval between checks.").
		Default(defaultInterval).DurationVar(&config.Interval)
	kingpin.Flag("metrics-address", "defines where to serve metrics").
		Default(defaultMetricsAddress).StringVar(&config.MetricsAddress)
	kingpin.Flag("pod-selector", "Pod selector specified by <namespace>:<key>=<value>,+.").
		SetValue(&config.PodSelectors)
	kingpin.Flag("pod-selector-configmap", "Name of configMap with pod selector definition. Must be in the same namespace.").
		StringVar(&config.ConfigMap)
	kingpin.Flag("ignore-node-label", "Ignore nodes based on label. Format <key>=<value>.").
		StringsVar(&config.IgnoreNodeLabels)
	kingpin.Flag("asg-lifecycle-hook", "Name of ASG lifecycle hook to trigger on node Ready.").
		StringVar(&config.ASGLifecycleHook)
	kingpin.Flag("enable-node-startup-metrics", "Enable node startup duration metrics.").
		BoolVar(&config.EnableNodeStartUpMetrics)
}

func main() {
	kingpin.Parse()

	var awsSession *session.Session
	var err error
	if config.ASGLifecycleHook != "" || config.EnableNodeStartUpMetrics {
		awsSession, err = pkgAWS.Session(aws.NewConfig())
		if err != nil {
			log.Fatalf("Failed to setup aws Session: %v", err)
		}
	}

	var hooks []Hook
	if config.ASGLifecycleHook != "" {
		hooks = append(hooks, NewASGLifecycleHook(awsSession, config.ASGLifecycleHook))
	}

	var startupObeserver NodeStartUpObeserver
	if config.EnableNodeStartUpMetrics {
		startupObeserver, err = NewASGNodeStartUpObserver(awsSession)
		if err != nil {
			log.Fatalf("Failed to setup observer: %v", err)
		}
	}

	ignoreLabels, err := parseNodeLabels(config.IgnoreNodeLabels)
	if err != nil {
		log.Fatal(err)
	}

	controller, err := NewNodeController(
		config.PodSelectors,
		ignoreLabels,
		config.Interval,
		config.ConfigMap,
		hooks,
		startupObeserver,
	)
	if err != nil {
		log.Fatal(err)
	}

	stopChan := make(chan struct{}, 1)
	go handleSigterm(stopChan)

	go serveMetrics(config.MetricsAddress)

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

func serveMetrics(address string) {
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(address, nil))
}
