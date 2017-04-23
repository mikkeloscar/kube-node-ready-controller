package main

import (
	"testing"

	"k8s.io/client-go/pkg/api/v1"
)

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
