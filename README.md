# Kube Node Ready Controller
[![Build Status](https://travis-ci.org/mikkeloscar/kube-node-ready-controller.svg?branch=master)](https://travis-ci.org/mikkeloscar/kube-node-ready-controller)
[![Coverage Status](https://coveralls.io/repos/github/mikkeloscar/kube-node-ready-controller/badge.svg)](https://coveralls.io/github/mikkeloscar/kube-node-ready-controller)

It is common to run a number of system pods (usually as DaemonSets) on each
node in a Kubernetes cluster in order to provide basic functionality. For
instance, you might want to run [kube2iam][kube2iam] to control AWS IAM access
for the services in your cluster, or you might run a
[logging-agent][logging-agent] to automatically ship logs to a central
location. Whatever your use case might be, you would expect these components to
run on all nodes, ideally before "normal" services are scheduled to the nodes.

By default in Kubernetes a node is considered `Ready`/`NotReady` based
on the node health independent of what system pods might be scheduled on the
node.

`kube-node-ready-controller` adds a layer on top to indicate whether a
node is ready for workloads based on a list of system pods which must be
running on the node before it is considered ready.

## How it works

The controller is configured with a list of pod selectors (namespace + labels)
and for each node it will check if the pods are scheduled and has status ready.
If all expected pods are ready it will make sure the node doesn't have the
[taint][taints-tolerations] `node.alpha.kubernetes.io/notReady-workload`. If
some expected pods aren't ready, it will make sure to set the taint on the
node.

## Setup

The `kube-node-ready-controller` can be run as a deployment in the cluster. See
[deployment.yaml](/docs/deployment.yaml).

To deploy it to your cluster modify the `--pod-selector` args to match your
system pods. The format for the selector is
`<namespace>:<labelKey>=<labelValue>,<labelKey2>=<labelValue2>`. Alternatively
you can set the flag `--pod-selector-configmap` and use a configMap to
configure the selectors ([full example](/docs/configmap.yaml)):

```yaml
selectors:
- namespace: kube-system
  labels:
    foo: bar
```

With this approach you can change the selectors at runtime, just by updating
the config map.

Once configured, deploy it by running:

```bash
$ kubectl apply -f docs/deployment.yaml
```

Note that we set the following toleration on the pod:

```yaml
tolerations:
- key: node.alpha.kubernetes.io/notReady-workload
  operator: Exists
  effect: NoSchedule
```

This is done to ensure that it can be scheduled even on nodes that are not
ready.

You must add the same toleration to all the system pods that should be
scheduled before the node is considered ready. If you fail to add the
toleration, the pod won't get scheduled and the node will thus never become
ready.

Lastly you must configure the nodes to have the `notReady-workload` taint when
they register with the cluster. This can be done by setting the flag
`--register-with-taints=node.alpha.kubernetes.io/notReady-workload=:NoSchedule`
on the `kubelet`.

You can also add the taint manually with `kubectl` to test it:

```bash
$ kubectl taint nodes <nodename> "node.alpha.kubernetes.io/notReady-workload=:NoSchedule"
```

## Hooks

As an extra feature `kube-node-ready-controller` has optional support for
triggering hooks when a node is marked as ready.

### AWS Autoscaling Lifecycle Hook

Trigger AWS Autoscaling Group lifecycle hook when node becomes ready. This can
be used to signal the Autoscaling Group that the node is in service.

Enable the hook with the flag `--asg-lifecycle-hook=<hook-name>`. This assumes
you have a hook with the defined name on the Autoscaling groups of all the
nodes managed by the controller.

## TODO

* [x] Make it possible to configure pod selectors via a config map.

* [ ] Instead of long polling the node list, add a Watch feature.


[kube2iam]: https://github.com/jtblin/kube2iam
[logging-agent]: https://github.com/zalando-incubator/kubernetes-log-watcher
[taints-tolerations]: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#taints-and-tolerations-beta-feature
