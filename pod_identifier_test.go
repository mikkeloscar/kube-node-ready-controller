package main

import "testing"

func TestPodIdentifierString(t *testing.T) {
	podIdentifiers := PodIdentifiers([]*PodIdentifier{
		{
			Namespace: "kube-system",
			Labels:    map[string]string{"key": "value"},
		},
	})
	expected := "kube-system:key=value"

	if podIdentifiers.String() != expected {
		t.Errorf("expected %s, got %s", expected, podIdentifiers.String())
	}

}

func TestSetPodIdentifierValue(t *testing.T) {
	for _, tc := range []struct {
		msg   string
		value string
		valid bool
	}{
		{
			msg:   "test valid identifier",
			value: "kube-system:application=skipper-ingress",
			valid: true,
		},
		{
			msg:   "test invalid identifier with missing labels",
			value: "kube-system",
			valid: false,
		},
		{
			msg:   "test invalid identifier with invalid label definition",
			value: "kube-system:key-value",
			valid: false,
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			podIdentifiers := PodIdentifiers([]*PodIdentifier{})
			err := podIdentifiers.Set(tc.value)
			if err != nil && tc.valid {
				t.Errorf("should not fail: %s", err)
			}

			if err == nil && !tc.valid {
				t.Error("expected failure")
			}
		})
	}
}

func TestPodIdentifierIsCumulative(t *testing.T) {
	podIdentifiers := PodIdentifiers([]*PodIdentifier{})
	if !podIdentifiers.IsCumulative() {
		t.Error("expected IsCumulative = true")
	}
}
