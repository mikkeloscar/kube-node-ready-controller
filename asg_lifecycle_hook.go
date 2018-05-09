package main

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
)

const (
	lifecycleActionContinue = "CONTINUE"
)

// Hook is an interface describing a hook which can be triggered given an
// instance id.
type Hook interface {
	Name() string
	Trigger(providerID string) error
}

// ASGLifecycleHook defines an ASG lifecycle hook to be triggered on node
// Ready.
type ASGLifecycleHook struct {
	hookName string
	svc      autoscalingiface.AutoScalingAPI
}

// NewASGLifecycleHook creates a new asg lifecycle hook.
func NewASGLifecycleHook(sess *session.Session, hookName string) *ASGLifecycleHook {
	return &ASGLifecycleHook{
		hookName: hookName,
		svc:      autoscaling.New(sess),
	}
}

// Name returns the hook name.
func (a *ASGLifecycleHook) Name() string {
	return a.hookName
}

// Trigger triggers a the ASG lifecycle hook for a given instance.
func (a *ASGLifecycleHook) Trigger(providerID string) error {
	instanceID, err := instanceIDFromProviderID(providerID)
	if err != nil {
		return err
	}

	// get ASG from instance-id
	instances := &autoscaling.DescribeAutoScalingInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceID),
		},
	}

	result, err := a.svc.DescribeAutoScalingInstances(instances)
	if err != nil {
		return err
	}

	if len(result.AutoScalingInstances) != 1 {
		return fmt.Errorf("expected 1 instance returned, got %d", len(result.AutoScalingInstances))
	}

	input := &autoscaling.CompleteLifecycleActionInput{
		AutoScalingGroupName:  result.AutoScalingInstances[0].AutoScalingGroupName,
		InstanceId:            aws.String(instanceID),
		LifecycleActionResult: aws.String(lifecycleActionContinue),
		LifecycleHookName:     aws.String(a.hookName),
	}

	_, err = a.svc.CompleteLifecycleAction(input)
	return err
}

// instanceIDFromProviderID extracts the EC2 instanceID from a Kubernetes
// ProviderID.
func instanceIDFromProviderID(providerID string) (string, error) {
	full := strings.TrimPrefix(providerID, "aws:///")
	split := strings.Split(full, "/")
	if len(split) != 2 {
		return "", fmt.Errorf("unexpected providerID format: %s", providerID)
	}

	return split[1], nil
}
