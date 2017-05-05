package main

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/pkg/api/v1"
)

func setupMockKubernetes(t *testing.T, node *v1.Node, config *v1.ConfigMap) kubernetes.Interface {
	client := fake.NewSimpleClientset()

	if node != nil {
		_, err := client.CoreV1().Nodes().Create(node)
		if err != nil {
			t.Error(err)
		}
	}

	if config != nil {
		_, err := client.CoreV1().ConfigMaps("kube-system").Create(config)
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

func TestRunOnce(t *testing.T) {
	for _, tc := range []struct {
		msg     string
		node    *v1.Node
		config  *v1.ConfigMap
		success bool
	}{
		{
			msg: "runOnce should succeed.",
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
			config: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config",
					Namespace: "kube-system",
				},
				Data: map[string]string{ConfigMapSelectorsKey: `selectors:
- namespace: kube-system
  labels:
    foo: bar`},
			},
			success: true,
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			controller := &NodeController{
				Interface: setupMockKubernetes(t, tc.node, tc.config),
				configMap: "",
			}
			if tc.config != nil {
				controller.configMap = tc.config.Name
			}

			err := controller.runOnce()
			if err != nil && tc.success {
				t.Errorf("should not fail: %s", err)
			}
		})
	}
}

func TestRun(t *testing.T) {
	stopCh := make(chan struct{}, 1)
	node := &v1.Node{
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
	}

	config := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config",
			Namespace: "kube-system",
		},
		Data: map[string]string{ConfigMapSelectorsKey: `selectors:
- namespace: kube-system
labels:
foo: bar`},
	}

	controller := &NodeController{
		Interface: setupMockKubernetes(t, node, config),
		configMap: config.Name,
	}

	go controller.Run(stopCh)
	stopCh <- struct{}{}
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
				Interface: setupMockKubernetes(t, nil, nil),
				selectors: tc.selectors,
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
				Interface: setupMockKubernetes(t, tc.node, nil),
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

func TestGetConfig(t *testing.T) {
	for _, tc := range []struct {
		msg     string
		config  *v1.ConfigMap
		success bool
	}{
		{
			msg: "valid config map should overwrite selectors",
			config: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config",
					Namespace: "kube-system",
				},
				Data: map[string]string{ConfigMapSelectorsKey: `selectors:
- namespace: kube-system
  labels:
    foo: bar`},
			},
			success: true,
		},
		{
			msg: "config map with invalid key should fail",
			config: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config",
					Namespace: "kube-system",
				},
				Data: map[string]string{"invalid": `selectors:
- namespace: kube-system
  labels:
    foo: bar`},
			},
			success: false,
		},
		{
			msg: "config map with invalid content should fail",
			config: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config",
					Namespace: "kube-system",
				},
				Data: map[string]string{ConfigMapSelectorsKey: `selectors`},
			},
			success: false,
		},
		{
			msg:     "no configMap exists should fail",
			config:  nil,
			success: false,
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			controller := &NodeController{
				Interface: setupMockKubernetes(t, nil, tc.config),
				configMap: "config",
			}

			err := controller.getConfig()
			if err != nil && tc.success {
				t.Errorf("should not fail: %s", err)
			}

			if err == nil && !tc.success {
				t.Error("expected failure")
			}

			// n, err := controller.CoreV1().Nodes().Get(tc.node.Name, metav1.GetOptions{})
			// if err != nil {
			// 	t.Errorf("should not fail: %s", err)
			// }

			// if tc.ready && hasTaint(n) {
			// 	t.Errorf("node should not have taint when ready")
			// }

			// if !tc.ready && !hasTaint(n) {
			// 	t.Errorf("node should have taint when not ready")
			// }
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
