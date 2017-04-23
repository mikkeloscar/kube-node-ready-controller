# Kube Node Ready Controller

Simple controller to add/remove `node.alpha.kubernetes.io.custom/notReady`
taint from node when it is considered ready or not ready based on expected
system pods running on the node.

## Setup

```yaml
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: kube-node-ready-controller
  namespace: kube-system
  labels:
    application: kube-node-ready-controller
    version: latest
spec:
  strategy:
    type: Recreate
  selector:
    matchLabels:
      application: kube-node-ready-controller
  template:
    metadata:
      labels:
        application: kube-node-ready-controller
        version: latest
      annotations:
        scheduler.alpha.kubernetes.io/critical-pod: ''
    spec:
      tolerations:
       - key: CriticalAddonsOnly
         operator: Exists
       - key: node.alpha.kubernetes.io.custom/notReady
         operator: Exists
         effect: NoSchedule
      containers:
      - name: kube-node-ready-controller
        image: mikkeloscar/kube-node-ready-controller:latest
        args:
        # format <namespace>:<labelKey>=<labelValue>,+
        - "--pod-identifier=kube-system:application=skipper-ingress"
        - "--pod-identifier=kube-system:application=kube2iam"
        - "--pod-identifier=kube-system:application=kube-proxy"
        - "--pod-identifier=kube-system:application=logging-agent"
        - "--pod-identifier=kube-system:application=prometheus-node-exporter"
        resources:
          limits:
            cpu: 200m
            memory: 200Mi
          requests:
            cpu: 10m
            memory: 25Mi
```

```bash
$ kubectl apply -f kube-node-ready-controller.yaml
```

## TODO

* [ ] Better logs
* [ ] config map
* [ ] Watch
