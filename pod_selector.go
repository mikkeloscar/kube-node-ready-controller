package main

import (
	"fmt"
	"strings"
)

// PodSelector consist of namespace and labels that can identify a Pod.
type PodSelector struct {
	Namespace string
	Labels    map[string]string
}

// PodSelectors is a list of PodSelector definitions.
type PodSelectors []*PodSelector

func (p PodSelectors) String() string {
	strs := make([]string, len(p))
	for i, t := range p {
		labels := make([]string, 0, len(t.Labels))
		for k, v := range t.Labels {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
		strs[i] = fmt.Sprintf("%s:%s", t.Namespace, strings.Join(labels, ","))
	}

	return strings.Join(strs, " - ")
}

// Set parses a pod selector string and adds it to the list.
func (p *PodSelectors) Set(value string) error {
	divide := strings.Split(value, ":")
	if len(divide) != 2 {
		return fmt.Errorf("invalid pod selector format")
	}

	namespace := divide[0]

	labelsStrs := strings.Split(divide[1], ",")
	labels := make(map[string]string, len(labelsStrs))
	for _, labelStr := range labelsStrs {
		kv := strings.Split(labelStr, "=")
		if len(kv) != 2 {
			return fmt.Errorf("invalid pod selector format")
		}
		labels[kv[0]] = kv[1]
	}

	*p = append(*p, &PodSelector{Namespace: namespace, Labels: labels})

	return nil
}

// IsCumulative always return true because it's allowed to call Set multiple
// times.
func (p PodSelectors) IsCumulative() bool {
	return true
}
