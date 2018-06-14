package main

import (
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	pkgAWS "github.com/mikkeloscar/kube-node-ready-controller/pkg/aws"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	defaultInterval              = "15s"
	defaultMetricsAddress        = ":7979"
	defaultTaintNodeNotReadyName = "node.alpha.kubernetes.io/notReady-workload"
)

var (
	config struct {
		Interval                 time.Duration
		MetricsAddress           string
		PodSelectors             PodSelectors
		NodeSelectors            Labels
		ConfigMap                string
		ASGLifecycleHook         string
		EnableNodeStartUpMetrics bool
		TaintNodeNotReadyName    string
		APIServer                *url.URL
	}
)

func init() {
	kingpin.Flag("interval", "Interval between checks.").
		Default(defaultInterval).DurationVar(&config.Interval)
	kingpin.Flag("apiserver", "API server url.").URLVar(&config.APIServer)
	kingpin.Flag("metrics-address", "defines where to serve metrics").
		Default(defaultMetricsAddress).StringVar(&config.MetricsAddress)
	kingpin.Flag("pod-selector", "Pod selector specified by <namespace>:<key>=<value>,+.").
		SetValue(&config.PodSelectors)
	kingpin.Flag("node-selector", "Node selector labels <key>=<value>,+.").
		SetValue(&config.NodeSelectors)
	kingpin.Flag("pod-selector-configmap", "Name of configMap with pod selector definition. Must be in the same namespace.").
		StringVar(&config.ConfigMap)
	kingpin.Flag("asg-lifecycle-hook", "Name of ASG lifecycle hook to trigger on node Ready.").
		StringVar(&config.ASGLifecycleHook)
	kingpin.Flag("enable-node-startup-metrics", "Enable node startup duration metrics.").
		BoolVar(&config.EnableNodeStartUpMetrics)
	kingpin.Flag("not-ready-taint-name", "Name of the taint set for not ready nodes.").
		StringVar(&config.TaintNodeNotReadyName)
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

	var kubeConfig *rest.Config
	if config.APIServer != nil {
		kubeConfig = &rest.Config{
			Host: config.APIServer.String(),
		}
	} else {
		kubeConfig, err = rest.InClusterConfig()
		if err != nil {
			log.Fatal(err)
		}
	}

	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		log.Fatal(err)
	}

	controller, err := NewNodeController(
		client,
		config.PodSelectors,
		config.NodeSelectors,
		config.TaintNodeNotReadyName,
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

func serveMetrics(address string) {
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(address, nil))
}
