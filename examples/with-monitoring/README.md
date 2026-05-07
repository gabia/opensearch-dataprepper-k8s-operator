# Example: Prometheus scraping + autoscaling

A cluster that exposes its metrics to Prometheus and scales itself
based on CPU utilization.

## Prerequisites

| Component | Required for | If missing |
|---|---|---|
| Prometheus Operator (CRDs `monitoring.coreos.com/v1`) | ServiceMonitor | The operator silently skips ServiceMonitor reconciliation. The cluster still runs fine; metrics just are not scraped. |
| metrics-server | HPA CPU readings | HPA stays at minReplicas because it cannot read CPU. |

## Apply

```sh
kubectl apply -f cluster.yaml
kubectl apply -f pipeline.yaml
```

## Verify

The cluster carries both an HPA and (if the CRD is installed) a
ServiceMonitor:

```sh
kubectl -n operator-lab get datapreppercluster monitored-prepper
kubectl -n operator-lab get hpa monitored-prepper
kubectl -n operator-lab get servicemonitor monitored-prepper 2>/dev/null \
    || echo "(ServiceMonitor CRD not installed; skip is normal)"
```

HPA picks up CPU readings within ~30 seconds of pods being ready:

```sh
kubectl -n operator-lab get hpa monitored-prepper -w
# NAME                REFERENCE                      TARGETS         MINPODS   MAXPODS   REPLICAS
# monitored-prepper   Deployment/monitored-prepper   cpu: 12%/70%    2         6         2
```

Drive load against the cluster's HTTP port (2021 in the sample
pipeline) and watch REPLICAS climb past 2 toward the target.

## Disabling either feature

Remove `spec.metrics` from the cluster spec to delete the ServiceMonitor.
Remove `spec.autoscaling` to delete the HPA and have the operator
manage replicas directly again.
