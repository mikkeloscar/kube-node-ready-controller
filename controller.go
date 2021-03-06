package main

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

const (
	// ConfigMapSelectorsKey defines the key name of the config map where
	// the pod selector definition is defined.
	ConfigMapSelectorsKey   = "pod_selectors"
	serviceAccountNamespace = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	maxConflictRetries      = 50
)

// NodeController updates the readiness taint of nodes based on expected
// resources defined by selectors.
type NodeController struct {
	kubernetes.Interface
	selectors             []*PodSelector
	nodeSelectorLabels    labels.Set
	interval              time.Duration
	configMap             string
	namespace             string
	nodeReadyHooks        []Hook
	nodeStartUpObserver   NodeStartUpObserver
	taintNodeNotReadyName string
}

// NewNodeController initializes a new NodeController.
func NewNodeController(client kubernetes.Interface, selectors []*PodSelector, nodeSelectorLabels map[string]string, taintNodeNotReadyName string, interval time.Duration, configMap string, hooks []Hook, nodeStartUpObserver NodeStartUpObserver) (*NodeController, error) {
	controller := &NodeController{
		Interface:             client,
		selectors:             selectors,
		nodeSelectorLabels:    labels.Set(nodeSelectorLabels),
		interval:              interval,
		configMap:             configMap,
		nodeReadyHooks:        hooks,
		nodeStartUpObserver:   nodeStartUpObserver,
		taintNodeNotReadyName: taintNodeNotReadyName,
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

	opts := metav1.ListOptions{
		LabelSelector: n.nodeSelectorLabels.String(),
	}

	nodes, err := n.CoreV1().Nodes().List(opts)
	if err != nil {
		return err
	}

	log.Infof("Checking %d nodes for readiness", len(nodes.Items))

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
	setNodeReadiness := func() error {
		updatedNode, err := n.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
		if err != nil {
			return backoff.Permanent(err)
		}

		// if ready, remove notReady taint if exists on the node
		if ready {
			var newTaints []v1.Taint
			for _, taint := range updatedNode.Spec.Taints {
				if taint.Key != n.taintNodeNotReadyName {
					newTaints = append(newTaints, taint)
				}
			}

			if len(newTaints) == len(updatedNode.Spec.Taints) {
				return nil
			}
			updatedNode.Spec.Taints = newTaints
		} else {
			if hasTaint(updatedNode, n.taintNodeNotReadyName) {
				return nil
			}

			taint := v1.Taint{
				Key:    n.taintNodeNotReadyName,
				Effect: v1.TaintEffectNoSchedule,
			}
			updatedNode.Spec.Taints = append(updatedNode.Spec.Taints, taint)
		}

		_, err = n.CoreV1().Nodes().Update(updatedNode)
		if err != nil {
			// automatically retry if there was a conflicting update.
			if errors.IsConflict(err) {
				return err
			}
			return backoff.Permanent(err)
		}

		if ready {
			log.WithFields(log.Fields{
				"action": "removed",
				"taint":  n.taintNodeNotReadyName,
				"node":   updatedNode.ObjectMeta.Name,
			}).Info("")

			if n.nodeStartUpObserver != nil {
				// observe node startup duration
				n.nodeStartUpObserver.ObserveNode(*updatedNode)
			}

			// trigger hooks on node ready.
			for _, hook := range n.nodeReadyHooks {
				err := hook.Trigger(updatedNode.Spec.ProviderID)
				if err != nil {
					log.Errorf("Failed to trigger hook '%s': %v", hook.Name(), err)
				}
			}
		} else {
			log.WithFields(log.Fields{
				"action": "added",
				"taint":  n.taintNodeNotReadyName,
				"node":   updatedNode.ObjectMeta.Name,
			}).Info("")
		}

		return nil
	}

	backoffCfg := backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), maxConflictRetries)
	return backoff.Retry(setNodeReadiness, backoffCfg)
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

// hasTaint returns true if the node has the taint.
func hasTaint(node *v1.Node, taintName string) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == taintName {
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
