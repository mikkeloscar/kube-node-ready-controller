package main

import (
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// TaintNodeNotReadyWorkload defines a taint key indicating that a node
	// is not ready to receive workloads.
	// This taint should be set on all nodes at startup and be removed by
	// the kube-node-ready-controller once the required system pods are
	// running on the node.
	TaintNodeNotReadyWorkload = "node.alpha.kubernetes.io/notReady-workload"
)

// NodeController updates the readiness taint of nodes based on expected
// resources.
type NodeController struct {
	kubernetes.Interface
	resources []*PodSelector
	interval  time.Duration
}

// NewNodeController initializes a new NodeController.
func NewNodeController(resources []*PodSelector, interval time.Duration) (*NodeController, error) {
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &NodeController{
		Interface: client,
		resources: resources,
		interval:  interval,
	}, nil
}

// Run runs the controller loop until it receives a stop signal over the stop
// channel.
func (n *NodeController) Run(stopChan <-chan struct{}) {
	for {
		nodes, err := n.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
			log.Error(err)
			goto next
		}

		for _, node := range nodes.Items {
			err = n.handleNode(&node)
			if err != nil {
				log.Error(err)
				continue
			}
		}
	next:
		select {
		case <-time.After(n.interval):
		case <-stopChan:
			log.Info("terminating main controller loop")
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

	readyResources := make([]*PodSelector, 0, len(n.resources))
	for _, identifier := range n.resources {
		for _, pod := range pods.Items {
			if pod.ObjectMeta.Namespace == identifier.Namespace &&
				containLabels(pod.ObjectMeta.Labels, identifier.Labels) {
				if podReady(&pod) {
					readyResources = append(readyResources, identifier)
				}
				break
			}
		}
	}

	if len(readyResources) != len(n.resources) {
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
		}
	} else { // else add the taint if the node is not ready
		hasTaint := func(node *v1.Node) bool {
			for _, taint := range node.Spec.Taints {
				if taint.Key == TaintNodeNotReadyWorkload {
					return true
				}
			}
			return false
		}

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
