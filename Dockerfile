FROM scratch

ADD build/linux/kube-node-ready-controller /

ENTRYPOINT ["/kube-node-ready-controller"]
