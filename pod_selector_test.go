package main

import "testing"

func TestPodSelectorString(t *testing.T) {
	PodSelectors := PodSelectors([]*PodSelector{
		{
			Namespace: "kube-system",
			Labels:    map[string]string{"key": "value"},
		},
	})
	expected := "kube-system:key=value"

	if PodSelectors.String() != expected {
		t.Errorf("expected %s, got %s", expected, PodSelectors.String())
	}

}

func TestSetPodSelectorValue(t *testing.T) {
	for _, tc := range []struct {
		msg   string
		value string
		valid bool
	}{
		{
			msg:   "test valid selector",
			value: "kube-system:application=skipper-ingress",
			valid: true,
		},
		{
			msg:   "test invalid selector with missing labels",
			value: "kube-system",
			valid: false,
		},
		{
			msg:   "test invalid selector with invalid label definition",
			value: "kube-system:key-value",
			valid: false,
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			podSelectors := PodSelectors([]*PodSelector{})
			err := podSelectors.Set(tc.value)
			if err != nil && tc.valid {
				t.Errorf("should not fail: %s", err)
			}

			if err == nil && !tc.valid {
				t.Error("expected failure")
			}
		})
	}
}

func TestPodSelectorIsCumulative(t *testing.T) {
	podSelectors := PodSelectors([]*PodSelector{})
	if !podSelectors.IsCumulative() {
		t.Error("expected IsCumulative = true")
	}
}

func TestReadSelectors(t *testing.T) {
	const data = `selectors:
- namespace: kube-system
  labels:
    foo: bar`

	selectors, err := ReadSelectors(data)
	if err != nil {
		t.Errorf("should not fail: %s", err)
	}

	if len(selectors) != 1 {
		t.Errorf("expected %d selectors, got %d", 1, len(selectors))
	}

	if selectors[0].Namespace != "kube-system" {
		t.Errorf("expected namespace '%s', got '%s'", "kube-system", selectors[0].Namespace)
	}

	if len(selectors[0].Labels) != 1 {
		t.Errorf("expected %d selectors, got %d", 1, len(selectors[0].Labels))
	}

	const invalidData = `selectors:
	`
	_, err = ReadSelectors(invalidData)
	if err == nil {
		t.Errorf("expected error")
	}
}
