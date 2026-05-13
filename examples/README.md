# Examples

Worked manifests for common DataPrepper operator patterns. Each folder
is self-contained with its own README.

| Folder | Demonstrates |
|---|---|
| [otlp-to-opensearch/](./otlp-to-opensearch/) | End-to-end OTLP receiver to OpenSearch with logs, raw spans, and service map pipelines. The original gabia-otel PoC. |
| [multi-replica/](./multi-replica/) | `replicas: 3` plus operator-managed peer-forwarder DNS discovery and headless Service. |
| [with-class/](./with-class/) | Sharing image, resources, scheduling, and labels across many clusters via a single `DataPrepperClass`. |
| [with-secret-auth/](./with-secret-auth/) | Injecting OpenSearch credentials via `spec.envFrom` and resolving `${OS_USER}` / `${OS_PASSWORD}` in pipeline YAML. |
| [with-monitoring/](./with-monitoring/) | Optional Prometheus `ServiceMonitor` and CPU-driven `HorizontalPodAutoscaler`. |

All examples target the `operator-lab` namespace by default; change
`metadata.namespace` to fit your environment.

## Prerequisites

- The opensearch-dataprepper-k8s-operator deployed (`make deploy IMG=...` from the
  repo root).
- cert-manager installed in the cluster, since the operator's
  validating webhook needs a TLS cert.

Specific examples have additional prerequisites; see each folder's
README for details.
