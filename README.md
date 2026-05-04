# dataprepper-operator

A Kubernetes operator for [OpenSearch Data Prepper](https://github.com/opensearch-project/data-prepper) — manage DataPrepper instances and their pipelines as native Kubernetes resources, with hot pipeline updates and no manual config-map plumbing.

[![Tests](https://github.com/pkeugine/dataprepper-operator/actions/workflows/test.yml/badge.svg)](https://github.com/pkeugine/dataprepper-operator/actions/workflows/test.yml)
[![Lint](https://github.com/pkeugine/dataprepper-operator/actions/workflows/lint.yml/badge.svg)](https://github.com/pkeugine/dataprepper-operator/actions/workflows/lint.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

## Why

Stock DataPrepper bundles all of its routing, processing, and sink configuration into a single `pipelines.yaml`. Adding or modifying any pipeline in production typically means editing that file and restarting the process — losing in-flight data, briefly halting telemetry ingest, and coordinating the change manually across replicas.

This operator splits that monolith in two:

- A **`DataPrepperCluster`** describes the runtime (image, replicas, server config, asset templates).
- One or more **`DataPrepperPipeline`** resources describe individual pipelines, each as its own Kubernetes object.

Adding a pipeline is `kubectl apply -f new-pipeline.yaml`. The operator merges all pipelines targeting a cluster into one ConfigMap, hashes the content, and rolls the cluster only when the content actually changed.

## Custom Resources

| Kind | Scope | Purpose |
|---|---|---|
| `DataPrepperClass` | Cluster | Optional template — image + default resource requirements. Like `StorageClass` for `PersistentVolumeClaim`. |
| `DataPrepperCluster` | Namespaced | A DataPrepper Deployment + Service. Optionally references a `DataPrepperClass` for defaults. |
| `DataPrepperPipeline` | Namespaced | A single pipeline definition (raw DataPrepper YAML). Targets a `DataPrepperCluster` by name. |

### `DataPrepperCluster` spec at a glance

```yaml
apiVersion: dataprepper.gabia.com/v1alpha1
kind: DataPrepperCluster
metadata:
  name: my-prepper
spec:
  image: opensearchproject/data-prepper:2.15.0   # or use classRef
  replicas: 2
  serverConfigMap: dataprepper-config            # optional, for data-prepper-config.yaml
  assetsConfigMap: dataprepper-assets            # optional, for /usr/share/data-prepper/assets/
  extraPorts:                                    # optional, merged with the default 4900/http
    - name: otlp
      containerPort: 21892
  # extraVolumes / extraVolumeMounts available as escape hatches
```

`serverConfigMap` and `assetsConfigMap` are first-class shortcuts for the two ConfigMaps every real DataPrepper deployment needs. Anything else can still be wired up via `extraVolumes` / `extraVolumeMounts` / `extraPorts` without forking the operator.

### `DataPrepperPipeline` spec at a glance

```yaml
apiVersion: dataprepper.gabia.com/v1alpha1
kind: DataPrepperPipeline
metadata:
  name: traces
spec:
  clusterRef: my-prepper          # which cluster this pipeline runs on
  pipelineYaml: |                 # raw DataPrepper pipeline YAML, unmodified
    traces-pipeline:
      source:
        otlp:
          port: 21892
      sink:
        - opensearch:
            hosts: [http://opensearch:9200]
            index: otel-v1-apm-span
```

## How it works

```
DataPrepperCluster        DataPrepperPipeline (×N)
        │                          │
        │                          │ collected by clusterRef
        ▼                          ▼
  Deployment           ConfigMap (<cluster>-pipelines)
  Service                       │
        ▲                       │ content hash → annotation
        └───────────────────────┘
              rolling restart only when content changed
```

- **`DataPrepperClusterReconciler`** owns the Deployment, Service, and pipeline ConfigMap for each cluster. It resolves image/resources from `DataPrepperClass` (if `classRef` is set) and reports `Pending` / `Ready` / `Degraded` in `.status.phase`.
- **`DataPrepperPipelineReconciler`** watches pipelines, the cluster, and the cluster's ConfigMap. On any change it rebuilds the merged pipeline content, writes it to the ConfigMap, and bumps a content-hash annotation on the Deployment's pod template — which triggers a rolling restart automatically. Identical content is a no-op, so the rolling restart only fires when something actually changed.
- **Finalizers** on each `DataPrepperPipeline` ensure that deleting a pipeline rebuilds the ConfigMap and rolls the cluster on the way out, preventing orphaned config.

## Quick start

### Prerequisites

- Go 1.24+
- Docker 17.03+
- kubectl 1.11+
- A Kubernetes 1.20+ cluster (the e2e tests target [kind](https://kind.sigs.k8s.io/))

### Install

Build and push the operator image, then install:

```sh
make docker-build docker-push IMG=<registry>/dataprepper-operator:tag
make install                                 # CRDs
make deploy IMG=<registry>/dataprepper-operator:tag
```

### Try it

The fastest way to see the three CRDs in motion:

```sh
kubectl apply -k config/samples/
kubectl get datapreppercluster,dataprepperpipeline,dataprepperclass
```

For a real, end-to-end OTLP→OpenSearch flow with five interconnected pipelines, see [`examples/otlp-to-opensearch/`](./examples/otlp-to-opensearch/).

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

Alpha (`v1alpha1`). API may change. The operator is in active PoC use against a single-node cluster ingesting OTLP traffic from a real otel-collector into OpenSearch.

## License

Apache License 2.0. See [`LICENSE`](./LICENSE).
