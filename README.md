# opensearch-dataprepper-k8s-operator

A Kubernetes operator for [OpenSearch Data Prepper](https://github.com/opensearch-project/data-prepper). It manages DataPrepper instances and their pipelines as native Kubernetes resources, with hot pipeline updates, no manual config-map plumbing, and pluggable peer-forwarder discovery for multi-replica deployments.

## Why

Stock DataPrepper bundles all of its routing, processing, and sink configuration into a single `pipelines.yaml`. Adding or modifying any pipeline in production typically means editing that file and restarting the process. That loses in-flight data, briefly halts telemetry ingest, and forces operators to coordinate the change manually across replicas.

This operator splits that monolith in two:

- A **`DataPrepperCluster`** describes the runtime (image, replicas, scheduling, secrets, observability).
- One or more **`DataPrepperPipeline`** resources describe individual pipelines, each as its own Kubernetes object.

Adding a pipeline is `kubectl apply -f new-pipeline.yaml`. The operator merges all pipelines targeting a cluster into one ConfigMap, hashes the content, and rolls the cluster only when the content actually changed.

## Custom Resources

| Kind | Scope | Purpose |
|---|---|---|
| `DataPrepperClass` | Cluster | Cluster-scoped template carrying image, default resources, scheduling, common labels. Like `StorageClass` for `PersistentVolumeClaim`. |
| `DataPrepperCluster` | Namespaced | A DataPrepper runtime: Deployment, Services, optional HPA and ServiceMonitor. Optionally references a class for defaults. |
| `DataPrepperPipeline` | Namespaced | A single structured pipeline definition. Targets a cluster by name. |

### `DataPrepperCluster` spec at a glance

```yaml
apiVersion: dataprepper.gabia.com/v1alpha1
kind: DataPrepperCluster
metadata:
  name: my-prepper
spec:
  image: opensearchproject/data-prepper:2.15.0   # or use classRef
  replicas: 3                                    # multi-replica works out of the box
  resources: {}                                  # optional, overrides class default

  # Optional ConfigMap mounts. When serverConfigMap is unset the operator
  # generates an appropriate data-prepper-config.yaml itself (local_node
  # for replicas=1, dns discovery for replicas>=2).
  serverConfigMap: my-prepper-config
  assetsConfigMap: my-prepper-assets

  # Inject env vars from Secrets or ConfigMaps. Pipeline YAML can use
  # ${VAR_NAME} substitution against these.
  envFrom:
    - secretRef:
        name: opensearch-credentials

  # Reconcile a Prometheus Operator ServiceMonitor when the CRD is
  # installed; otherwise silently skipped.
  metrics:
    serviceMonitor: true

  # CPU-driven HorizontalPodAutoscaler. The HPA owns replicas while set.
  autoscaling:
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70

  # Escape hatches for anything the operator does not model directly.
  extraPorts: []
  extraVolumes: []
  extraVolumeMounts: []
```

### `DataPrepperPipeline` spec at a glance

```yaml
apiVersion: dataprepper.gabia.com/v1alpha1
kind: DataPrepperPipeline
metadata:
  name: traces
spec:
  clusterRef: my-prepper
  yamlKey: traces-pipeline
  pipeline:
    source:
      otlp:
        port: 21892
    sink:
      - opensearch:
          hosts: [https://opensearch:9200]
          index: otel-v1-apm-span
          username: ${OS_USER}
          password: ${OS_PASSWORD}
```

`yamlKey` becomes the top-level key in the generated Data Prepper YAML. A validating webhook rejects invalid pipeline definitions, duplicate keys within a cluster, and definitions missing `source` or `sink`.

> **Migration note:** `pipelineYaml` is no longer supported. Recreate existing `DataPrepperPipeline` resources with `yamlKey` and `pipeline` before upgrading the CRD.

## What the operator handles

- **Hot pipeline updates** without downtime via content-hashed rolling restarts.
- **Multi-replica** runtime with operator-managed peer-forwarder DNS discovery and headless Service.
- **Status-driven UX**: standard `Ready` / `Progressing` / `Degraded` conditions on Cluster, plus a `Healthy` condition on Pipeline that surfaces actual pod boot errors (CrashLoopBackOff reasons, image pull failures, OOM kills).
- **Secret-based credential injection** for sinks and sources.
- **Validating webhook** for pipeline shape, served via cert-manager-issued certs.
- **Optional Prometheus ServiceMonitor** when the CRD is present.
- **Optional HPA** keyed off CPU utilization.
- **Cluster-scoped templates** through `DataPrepperClass` for org-wide image and scheduling defaults.

See [`examples/`](./examples/) for self-contained manifests covering each capability.

## How it works

```
DataPrepperCluster                  DataPrepperPipeline (xN)
   |                                       |
   v                                       v collected by clusterRef
Deployment                          ConfigMap (<cluster>-pipelines)
Services (4900 + headless)               |
HPA / ServiceMonitor (optional)          | content hash -> annotation
Auto peer-config (when needed)           |
   ^_______________________________________|
       rolling restart only when content changed
```

- **`DataPrepperClusterReconciler`** owns the Deployment, Services, pipeline ConfigMap, and (when configured) HPA, ServiceMonitor, and operator-managed server config. It resolves image/resources/scheduling from `DataPrepperClass` when `classRef` is set.
- **`DataPrepperPipelineReconciler`** watches pipelines, the cluster, and the cluster's ConfigMap. It renders each pipeline's `yamlKey` and structured `pipeline` definition into the merged ConfigMap, then bumps a content-hash annotation on the Deployment Pod template. Identical content is a no-op; the rolling restart only fires when something actually changed. The reconciler also lists pods to surface DataPrepper boot errors back onto Pipeline status.
- **Finalizers** on each Pipeline ensure that deleting a pipeline rebuilds the ConfigMap and rolls the cluster on the way out, preventing orphaned config.

## Quick start

### Prerequisites

- Go 1.25+
- Docker (or any OCI image builder)
- kubectl 1.20+
- A Kubernetes 1.20+ cluster (the e2e tests target [kind](https://kind.sigs.k8s.io/))
- [cert-manager](https://cert-manager.io/) installed in the cluster. The operator's validating webhook requires it.

### Install

```sh
make docker-build docker-push IMG=<registry>/opensearch-dataprepper-k8s-operator:tag
make install                                 # CRDs
make deploy IMG=<registry>/opensearch-dataprepper-k8s-operator:tag
```

### Try it

The fastest way to see the three CRDs in motion:

```sh
kubectl apply -k config/samples/
kubectl get datapreppercluster,dataprepperpipeline,dataprepperclass
```

For real-world setups, browse [`examples/`](./examples/):

- `otlp-to-opensearch/`: end-to-end OTLP receiver with logs, raw spans, service map.
- `multi-replica/`: replicas=3 with auto peer-forwarder DNS discovery.
- `with-class/`: shared template for many clusters.
- `with-secret-auth/`: OpenSearch credentials from a Secret.
- `with-monitoring/`: ServiceMonitor and CPU-driven HPA.

### Uninstall

```sh
kubectl delete -k config/samples/
make undeploy
make uninstall
```

## Development

```sh
make generate manifests   # regenerate deepcopy + CRDs after editing api/
make lint                 # golangci-lint
make test                 # unit / envtest
make test-e2e             # full e2e against a kind cluster
make run                  # run the controller locally against the current kubectl context
```

See [`AGENTS.md`](./AGENTS.md) for the project layout and rules around generated files.

## Project status

Alpha (`v1alpha1`). API may still change. The operator is in active PoC use against a multi-node Kubernetes cluster ingesting OTLP traffic from a real otel-collector into OpenSearch, with the validating webhook backed by cert-manager-issued certs.
