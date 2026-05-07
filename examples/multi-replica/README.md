# Example: multi-replica with peer-forwarder DNS discovery

Three DataPrepper instances share the load. Peers find each other through
Kubernetes DNS so they can route events to the right instance.

## What the operator does

When `spec.replicas` is greater than one and `spec.serverConfigMap` is not
set, the operator:

1. Creates a ConfigMap `ha-prepper-peer-config` with
   `peer_forwarder.discovery_mode: dns` pointed at
   `ha-prepper-headless.<namespace>.svc.cluster.local`.
2. Reconciles a headless Service `ha-prepper-headless` (clusterIP: None)
   so the DNS name resolves to all pod IPs.
3. Stamps a content-hash annotation onto the Deployment Pod template, so
   any future change to peer-config or replicas count rolls the pods.

## Apply

```sh
kubectl apply -f cluster.yaml
kubectl apply -f pipeline.yaml
kubectl -n operator-lab get datapreppercluster ha-prepper -w
```

## Verify

The cluster moves to `Ready` once all three pods report ready:

```sh
kubectl -n operator-lab get datapreppercluster ha-prepper
# NAME         PHASE   READY   DESIRED   PIPELINES   AGE
# ha-prepper   Ready   3       3         1           2m
```

Confirm DNS resolves to all three pod IPs:

```sh
kubectl -n operator-lab run --rm -it dns-debug --image=busybox --restart=Never -- \
  nslookup ha-prepper-headless.operator-lab.svc.cluster.local
# Server:    10.96.0.10
# Address 1: 10.96.0.10 kube-dns.kube-system.svc.cluster.local
# Name:      ha-prepper-headless.operator-lab.svc.cluster.local
# Address 1: 192.168.x.x
# Address 2: 192.168.y.y
# Address 3: 192.168.z.z
```

## Override

If you prefer to manage the server config yourself (TLS, custom buffer
sizes, etc.), set `spec.serverConfigMap` on the Cluster. The operator
deletes the auto-managed `<cluster>-peer-config` and stops touching server
config; the headless Service still gets reconciled.
