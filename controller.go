package main

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	// TaintNodeNotReadyWorkload defines a taint key indicating that a node
	// is not ready to receive workloads.
	// This taint should be set on all nodes at startup and be removed by
	// the kube-node-ready-controller once the required system pods are
	// running on the node.
	TaintNodeNotReadyWorkload = "node.alpha.kubernetes.io/notReady-workload"
	// ConfigMapSelectorsKey defines the key name of the config map where
	// the pod selector definition is defined.
	ConfigMapSelectorsKey   = "pod_selectors"
	serviceAccountNamespace = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

// NodeController updates the readiness taint of nodes based on expected
// resources defined by selectors.
type NodeController struct {
	kubernetes.Interface
	selectors            []*PodSelector
	ignoreNodeLabels     map[string]string
	interval             time.Duration
	configMap            string
	namespace            string
	nodeReadyHooks       []Hook
	nodeStartUpObeserver NodeStartUpObeserver
}

// NewNodeController initializes a new NodeController.
func NewNodeController(selectors []*PodSelector, ignoreNodeLabels map[string]string, interval time.Duration, configMap string, hooks []Hook, nodeStartUpObeserver NodeStartUpObeserver) (*NodeController, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	controller := &NodeController{
		Interface:            client,
		selectors:            selectors,
		ignoreNodeLabels:     ignoreNodeLabels,
		interval:             interval,
		configMap:            configMap,
		nodeReadyHooks:       hooks,
		nodeStartUpObeserver: nodeStartUpObeserver,
	}

	if controller.configMap != "" {
		// get Current Namespace
		data, err := ioutil.ReadFile(serviceAccountNamespace)
		if err != nil {
			return nil, err
		}

		if ns := strings.TrimSpace(string(data)); len(ns) > 0 {
			controller.namespace = ns
		}
	}

	return controller, nil
}

func (n *NodeController) runOnce() error {
	// update selectors based on config map.
	if n.configMap != "" {
		err := n.getConfig()
		if err != nil {
			return err
		}
	}

	nodes, err := n.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, node := range nodes.Items {
		err = n.handleNode(&node)
		if err != nil {
			log.Error(err)
			continue
		}
	}

	return nil
}

// Run runs the controller loop until it receives a stop signal over the stop
// channel.
func (n *NodeController) Run(stopChan <-chan struct{}) {
	for {
		err := n.runOnce()
		if err != nil {
			log.Error(err)
		}

		select {
		case <-time.After(n.interval):
		case <-stopChan:
			log.Info("Terminating main controller loop.")
			return
		}
	}
}

// handleNode checks if a node is ready and updates the notReady taint
// accordingly.
func (n *NodeController) handleNode(node *v1.Node) error {
	if containLabels(node.Labels, n.ignoreNodeLabels) {
		log.WithFields(log.Fields{
			"node":   node.Name,
			"labels": n.ignoreNodeLabels,
		}).Info("Ignoring node because of ignore labels.")
		return nil
	}

	ready, err := n.nodeReady(node)
	if err != nil {
		return err
	}

	err = n.setNodeReady(node, ready)
	if err != nil {
		return err
	}

	return nil
}

// nodeReady checks if the required pods are scheduled on the node and has
// status ready.
func (n *NodeController) nodeReady(node *v1.Node) (bool, error) {
	opts := metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", node.ObjectMeta.Name),
	}

	pods, err := n.CoreV1().Pods(v1.NamespaceAll).List(opts)
	if err != nil {
		return false, err
	}

	readyResources := make([]*PodSelector, 0, len(n.selectors))
	for _, identifier := range n.selectors {
		for _, pod := range pods.Items {
			if pod.ObjectMeta.Namespace == identifier.Namespace &&
				containLabels(pod.ObjectMeta.Labels, identifier.Labels) {
				if podReady(&pod) {
					readyResources = append(readyResources, identifier)
				} else {
					// TODO: find all not ready pods
					log.WithFields(log.Fields{
						"pod":       pod.Name,
						"namespace": pod.Namespace,
						"node":      node.Name,
					}).Warn("Pod not ready.")
				}
				break
			}
		}
	}

	if len(readyResources) != len(n.selectors) {
		return false, nil
	}

	return true, nil
}

// setNodeReady sets node taint macthing ready value. E.g. sets NotReady taint
// if ready is false, and removes the taint (if exists) when ready is true.
func (n *NodeController) setNodeReady(node *v1.Node, ready bool) error {
	// if ready, remove notReady taint if exists on the node
	if ready {
		var newTaints []v1.Taint
		for _, taint := range node.Spec.Taints {
			if taint.Key != TaintNodeNotReadyWorkload {
				newTaints = append(newTaints, taint)
			}
		}

		if len(newTaints) != len(node.Spec.Taints) {
			node.Spec.Taints = newTaints
			_, err := n.CoreV1().Nodes().Update(node)
			if err != nil {
				return err
			}
			log.WithFields(log.Fields{
				"action": "removed",
				"taint":  TaintNodeNotReadyWorkload,
				"node":   node.ObjectMeta.Name,
			}).Info("")

			// observe node startup duration
			n.nodeStartUpObeserver.ObeserveNode(*node)

			// trigger hooks on node ready.
			for _, hook := range n.nodeReadyHooks {
				err := hook.Trigger(node.Spec.ProviderID)
				if err != nil {
					log.Errorf("Failed to trigger hook '%s': %v", hook.Name(), err)
				}
			}
		}
	} else { // else add the taint if the node is not ready
		if !hasTaint(node) {
			taint := v1.Taint{
				Key:    TaintNodeNotReadyWorkload,
				Effect: v1.TaintEffectNoSchedule,
			}
			node.Spec.Taints = append(node.Spec.Taints, taint)
			_, err := n.CoreV1().Nodes().Update(node)
			if err != nil {
				return err
			}
			log.WithFields(log.Fields{
				"action": "added",
				"taint":  TaintNodeNotReadyWorkload,
				"node":   node.ObjectMeta.Name,
			}).Info("")
		}
	}

	return nil
}

// getConfig gets a selector config from a config map.
func (n *NodeController) getConfig() error {
	configMap, err := n.CoreV1().ConfigMaps(n.namespace).Get(n.configMap, metav1.GetOptions{})
	if err != nil {
		return err
	}

	data, ok := configMap.Data[ConfigMapSelectorsKey]
	if !ok {
		return fmt.Errorf("expected key '%s' not present in config map", ConfigMapSelectorsKey)
	}

	selectors, err := ReadSelectors(data)
	if err != nil {
		return err
	}

	n.selectors = selectors
	return nil
}

// hasTaint returns true if the node has the taint TaintNodeNotReadyWorkload.
func hasTaint(node *v1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == TaintNodeNotReadyWorkload {
			return true
		}
	}
	return false
}

// containLabels reports whether expectedLabels are in labels.
func containLabels(labels, expectedLabels map[string]string) bool {
	for key, val := range expectedLabels {
		if v, ok := labels[key]; !ok || v != val {
			return false
		}
	}
	return true
}

// podReady returns true if all containers in the pod are ready.
func podReady(pod *v1.Pod) bool {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if !containerStatus.Ready {
			return false
		}
	}
	return true
}
