# Kube Node Ready Controller
[![Build Status](https://travis-ci.org/mikkeloscar/kube-node-ready-controller.svg?branch=master)](https://travis-ci.org/mikkeloscar/kube-node-ready-controller)

Simple controller to add/remove `node.alpha.kubernetes.io/notReady-workload`
taint from node when it is considered ready or not ready based on expected
system pods running on the node.

## Setup

See example [deployment.yaml](/Docs/deployment.yaml).

```bash
$ kubectl apply -f Docs/deployment.yaml
```

## TODO

* [ ] Better logs
* [ ] config map
* [ ] Watch
