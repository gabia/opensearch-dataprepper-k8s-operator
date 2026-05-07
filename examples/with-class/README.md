# Example: shared template via DataPrepperClass

A `DataPrepperClass` is to `DataPrepperCluster` what `StorageClass` is to
`PersistentVolumeClaim`: a cluster-scoped template that holds the values
every Cluster in your org should share.

## What the class carries

This example defines `gabia-otel-standard`, which sets:

- `image`: pinned 2.15.0 across all teams
- `resources`: sensible defaults
- `podLabels`: org-wide tagging (team, cost-center)
- `nodeSelector` + `tolerations`: schedule on the data-pipeline node pool

## How clusters use it

Two clusters reference the class via `spec.classRef`. They only declare
the fields that differ:

- `traces-prepper`: stays on every default
- `logs-prepper`: bumps replicas to 2 and overrides resources to ask for
  more memory

Cluster fields override Class values where they overlap. Pod-level fields
(nodeSelector, tolerations, podLabels, podAnnotations) currently come
from the Class only. Operator-managed pod labels (`app=<cluster>`)
always win.

## Apply

```sh
kubectl apply -f class.yaml
kubectl apply -f clusters.yaml
kubectl -n operator-lab get datapreppercluster
```

## Updating the class

Editing `class.yaml` and re-applying propagates the change to every
Cluster on the next reconcile (a few seconds). Useful for things like
bumping the DataPrepper image version across the org with a single edit.
