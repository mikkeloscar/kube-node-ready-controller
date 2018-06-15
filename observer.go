package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
)

// NodeStartUpObserver describes an observer which can observe the startup
// time duration of a node.
type NodeStartUpObserver interface {
	ObserveNode(node v1.Node)
}

// ASGNodeStartUpObserver is a node startup duration oberserver which determines
// the statup time duration based on ec2 instance launch time.
type ASGNodeStartUpObserver struct {
	ec2Client              ec2iface.EC2API
	nodesObserved         sync.Map
	startUpDurationSeconds prometheus.Summary
}

// NewASGNodeStartUpObserver registers a prometheus summary vec and returns a
// ASGNodeStartUpObserver.
func NewASGNodeStartUpObserver(sess *session.Session) (*ASGNodeStartUpObserver, error) {
	startUpDurationSeconds := prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name:       "startup_duration_seconds",
			Help:       "The node startup latencies in seconds.",
			Subsystem:  "node",
			Objectives: prometheus.DefObjectives,
		},
	)

	err := prometheus.Register(startUpDurationSeconds)
	if err != nil {
		return nil, err
	}

	return &ASGNodeStartUpObserver{
		ec2Client:              ec2.New(sess),
		startUpDurationSeconds: startUpDurationSeconds,
	}, nil
}

// ObserveNode observes the node startup time duration based on the launch
// time of the underlying ec2 instance.
// The observation is executed in a go routine to not block the caller.
func (o *ASGNodeStartUpObserver) ObserveNode(node v1.Node) {
	go func() {
		now := time.Now().UTC()

		if _, ok := o.nodesObserved.Load(node.Name); ok {
			log.Infof("Ignoring node %s already observed", node.Name)
			return
		}

		launchTime, err := o.nodeLaunchTime(node.Spec.ProviderID)
		if err != nil {
			log.Errorf("Failed to get node launch time: %v", err)
			return
		}

		o.startUpDurationSeconds.Observe(now.Sub(launchTime).Seconds())

		// record that node was observed
		o.nodesObserved.Store(node.Name, nil)
	}()
}

// nodeLaunchTime get the startup time of the underlying ec2 instance.
func (o *ASGNodeStartUpObserver) nodeLaunchTime(providerID string) (time.Time, error) {
	instanceID, err := instanceIDFromProviderID(providerID)
	if err != nil {
		return time.Time{}, fmt.Errorf("Failed to get instanceID for node: %v", err)
	}

	params := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(instanceID)},
	}

	resp, err := o.ec2Client.DescribeInstances(params)
	if err != nil {
		return time.Time{}, fmt.Errorf("Failed to describe instance: %v", err)
	}

	if len(resp.Reservations) != 1 {
		return time.Time{}, fmt.Errorf("Expected one reservation, got %d", len(resp.Reservations))
	}

	if len(resp.Reservations[0].Instances) != 1 {
		return time.Time{}, fmt.Errorf("Expected one instance, got %d", len(resp.Reservations[0].Instances))
	}

	return aws.TimeValue(resp.Reservations[0].Instances[0].LaunchTime), nil
}
