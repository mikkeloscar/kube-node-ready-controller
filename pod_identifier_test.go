package main

import "testing"

func TestSetPodIdentifierValue(t *testing.T) {
	podIdentifiers := PodIdentifiers([]*PodIdentifier{})
	err := podIdentifiers.Set("kube-system:application=skipper-ingress")
	if err != nil {
		t.Errorf("should not fail: %s", err)
	}
}
