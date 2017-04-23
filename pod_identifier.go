package main

import (
	"fmt"
	"strings"
)

// PodIdentifier consist of namespace and labels that can identify a Pod.
type PodIdentifier struct {
	Namespace string
	Labels    map[string]string
}

// PodIdentifiers is a list of PodIdentifier definitions.
type PodIdentifiers []*PodIdentifier

func (p PodIdentifiers) String() string {
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

// Set parses a pod identifier string and adds it to the list.
func (p *PodIdentifiers) Set(value string) error {
	divide := strings.Split(value, ":")
	if len(divide) != 2 {
		return fmt.Errorf("invalid pod identifier format")
	}

	namespace := divide[0]

	labelsStrs := strings.Split(divide[1], ",")
	labels := make(map[string]string, len(labelsStrs))
	for _, labelStr := range labelsStrs {
		kv := strings.Split(labelStr, "=")
		if len(kv) != 2 {
			return fmt.Errorf("invalid pod identifier format")
		}
		labels[kv[0]] = kv[1]
	}

	*p = append(*p, &PodIdentifier{Namespace: namespace, Labels: labels})

	return nil
}

// IsCumulative always return true because it's allowed to call Set multiple
// times.
func (p PodIdentifiers) IsCumulative() bool {
	return true
}
