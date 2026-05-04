# Example: OTLP → DataPrepper → OpenSearch

End-to-end example that ingests OpenTelemetry traces and logs over OTLP/gRPC,
processes them with DataPrepper, and writes to OpenSearch indices ready for
the OpenSearch Dashboards Trace Analytics plugin.

```
otel-collector (or telemetrygen)
        │ OTLP/gRPC
        ▼
NodePort 31892 (gabia-otel-poc-otlp)
        │
        ▼
DataPrepper (port 21892)
   otlp source → route by event type
     ├─ LOG   → otel-logs-pipeline       → logs-otel-v1
     └─ TRACE → fan-out
                ├─ traces-raw-pipeline   → otel-v1-apm-span
                └─ service-map-pipeline  → otel-v2-apm-service-map
        │
        ▼
OpenSearch (opensearch-cluster-master.opensearch-system:9200)
```

## Prerequisites

- A Kubernetes cluster with the dataprepper-operator installed (`make deploy`
  from the repo root).
- An OpenSearch cluster reachable at
  `http://opensearch-cluster-master.opensearch-system:9200`. The
  [opensearch Helm chart](https://github.com/opensearch-project/helm-charts)
  in `opensearch-system` namespace works as-is.
- DataPrepper 2.15+ image pullable in the cluster (`opensearchproject/data-prepper:2.15.0`).
  Earlier versions do not have the `otel_apm_service_map` plugin.

## Files

| File | Purpose |
|---|---|
| `01-namespace.yaml` | Creates the `operator-lab` namespace |
| `02-server-config.yaml` | DataPrepper server config (peer-forwarder set to `local_node` for single-replica mode) |
| `03-cluster.yaml` | `DataPrepperCluster` — references the server config + assets ConfigMap, exposes OTLP ports |
| `04-pipeline.yaml` | `DataPrepperPipeline` with five sub-pipelines (otlp router, logs, traces fan-out, raw spans, service map) |
| `05-otlp-service.yaml` | NodePort service for external OTLP access |

## Asset ConfigMap (one-time setup)

The pipeline references index templates and ISM policies under
`/usr/share/data-prepper/assets/`. These come from the DataPrepper release —
copy them out of the upstream container, then create the ConfigMap:

```sh
docker run --rm --entrypoint sh opensearchproject/data-prepper:2.15.0 \
  -c 'tar -C /usr/share/data-prepper/assets -cf - .' | tar -C ./assets -xf -

kubectl -n operator-lab create configmap dataprepper-assets \
  --from-file=./assets
```

The pipeline expects these six files:
- `logs-otel-v1-index-standard-template.json`
- `logs-policy-with-ism-template.json`
- `otel-v1-apm-span-index-standard-template.json`
- `raw-span-policy-with-ism-template.json`
- `otel-v2-apm-service-map-index-template.json`
- `otel-v2-apm-service-map-policy-with-ism-template.json`

## Apply

```sh
kubectl apply -f .
kubectl -n operator-lab get datapreppercluster,datapreppercluster -w
```

The cluster moves through `Pending` → `Ready` once the DataPrepper Pod is up.
Pipeline status becomes `Applied` once the pipeline ConfigMap has been
written and a rolling restart has been triggered.

## Smoke test

Send 50 fake traces with [telemetrygen](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/cmd/telemetrygen):

```sh
telemetrygen traces \
  --otlp-endpoint <node-ip>:31892 \
  --otlp-insecure \
  --traces 50
```

Then check OpenSearch:

```sh
curl -s http://<opensearch>:9200/otel-v1-apm-span/_count
curl -s http://<opensearch>:9200/otel-v2-apm-service-map/_count
```

Open OpenSearch Dashboards → **Discover** → create an index pattern
`otel-v1-apm-span-*` to browse spans, or use the **Trace Analytics** plugin
(under Observability) which understands both indices natively.

## Updating the pipeline without downtime

Edit `04-pipeline.yaml` (or apply a new `DataPrepperPipeline` for the same
`clusterRef`) and re-apply. The operator rebuilds the cluster's pipeline
ConfigMap and triggers a rolling restart of the DataPrepper Deployment via
a content-hash annotation — no manual intervention, no orphaned config.
