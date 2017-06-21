package main

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
)

const (
	lifecycleActionContinue = "CONTINUE"
)

// Hook is an interface describing a hook which can be triggered given an
// instance id.
type Hook interface {
	Name() string
	Trigger(instance string) error
}

// autoscalingAPI defines the subset of the AWS Autoscaling API used to
// implement the hook. It helps mocking AWS calls in tests.
type autoscalingAPI interface {
	DescribeAutoScalingInstances(input *autoscaling.DescribeAutoScalingInstancesInput) (*autoscaling.DescribeAutoScalingInstancesOutput, error)
	CompleteLifecycleAction(input *autoscaling.CompleteLifecycleActionInput) (*autoscaling.CompleteLifecycleActionOutput, error)
}

// ASGLifecycleHook defines an ASG lifecycle hook to be triggered on node
// Ready.
type ASGLifecycleHook struct {
	hookName string
	svc      autoscalingAPI
}

// NewASGLifecycleHook creates a new asg lifecycle hook.
func NewASGLifecycleHook(hookName string) *ASGLifecycleHook {
	return &ASGLifecycleHook{
		hookName: hookName,
		svc:      autoscaling.New(session.New()),
	}
}

// Name returns the hook name.
func (a *ASGLifecycleHook) Name() string {
	return a.hookName
}

// Trigger triggers a the ASG lifecycle hook for a given instance.
func (a *ASGLifecycleHook) Trigger(instance string) error {
	// get ASG from instance-id
	instances := &autoscaling.DescribeAutoScalingInstancesInput{
		InstanceIds: []*string{
			aws.String(instance),
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
		InstanceId:            aws.String(instance),
		LifecycleActionResult: aws.String(lifecycleActionContinue),
		LifecycleHookName:     aws.String(a.hookName),
	}

	_, err = a.svc.CompleteLifecycleAction(input)
	return err
}
