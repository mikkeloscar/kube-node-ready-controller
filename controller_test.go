package main

import (
	"sync"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/pkg/api/v1"
)

func setupMockKubernetes(t *testing.T, node *v1.Node) kubernetes.Interface {
	client := fake.NewSimpleClientset()

	if node != nil {
		_, err := client.CoreV1().Nodes().Create(node)
		if err != nil {
			t.Error(err)
		}
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
			Labels:    map[string]string{"foo": "bar"},
		},
	}

	_, err := client.CoreV1().Pods(pod.Namespace).Create(pod)
	if err != nil {
		t.Error(err)
	}
	return client
}

func TestNodeReady(t *testing.T) {
	for _, tc := range []struct {
		msg       string
		selectors []*PodSelector
		ready     bool
	}{
		{
			msg: "node should be ready when pod is found",
			selectors: []*PodSelector{
				{
					Namespace: "default",
					Labels:    map[string]string{"foo": "bar"},
				},
			},
			ready: true,
		},
		{
			msg: "node should not be ready when pod is not found",
			selectors: []*PodSelector{
				{
					Namespace: "default",
					Labels:    map[string]string{"foo": "baz"},
				},
			},
			ready: false,
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			controller := &NodeController{
				Interface: setupMockKubernetes(t, nil),
				selectors: tc.selectors,
				RWMutex:   &sync.RWMutex{},
			}
			ready, _ := controller.nodeReady(&v1.Node{})

			if ready != tc.ready {
				t.Errorf("expected ready %t, got %t", tc.ready, ready)
			}
		})
	}
}

func TestSetNodeReady(t *testing.T) {
	for _, tc := range []struct {
		msg   string
		node  *v1.Node
		ready bool
	}{
		{
			msg: "taint should be removed when node is ready",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{
						{
							Key: TaintNodeNotReadyWorkload,
						},
						{
							Key: "foo",
						},
					},
				},
			},
			ready: true,
		},
		{
			msg: "taint should be added when node is not ready",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{
						{
							Key: "foo",
						},
					},
				},
			},
			ready: false,
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			controller := &NodeController{
				Interface: setupMockKubernetes(t, tc.node),
			}
			_ = controller.setNodeReady(tc.node, tc.ready)

			n, err := controller.CoreV1().Nodes().Get(tc.node.Name, metav1.GetOptions{})
			if err != nil {
				t.Errorf("should not fail: %s", err)
			}

			if tc.ready && hasTaint(n) {
				t.Errorf("node should not have taint when ready")
			}

			if !tc.ready && !hasTaint(n) {
				t.Errorf("node should have taint when not ready")
			}
		})
	}
}

func TestContainLabels(t *testing.T) {
	labels := map[string]string{
		"foo": "bar",
	}

	expected := map[string]string{
		"foo": "bar",
	}

	if !containLabels(labels, expected) {
		t.Errorf("expected %s to be contained in %s", expected, labels)
	}

	notExpected := map[string]string{
		"foo": "baz",
	}

	if containLabels(labels, notExpected) {
		t.Errorf("did not expect %s to be contained in %s", notExpected, labels)
	}
}

func TestPodReady(t *testing.T) {
	pod := &v1.Pod{
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Ready: true,
				},
			},
		},
	}

	if !podReady(pod) {
		t.Error("expected pod to be ready")
	}

	pod.Status.ContainerStatuses[0].Ready = false

	if podReady(pod) {
		t.Error("expected pod to not be ready")
	}
}
