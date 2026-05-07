# Example: OpenSearch sink with credentials from a Secret

How to get the OpenSearch user and password to DataPrepper without
checking them into the pipeline YAML.

## Mechanism

1. A Kubernetes Secret holds `OS_USER` and `OS_PASSWORD` keys.
2. `DataPrepperCluster.spec.envFrom` includes `secretRef: { name: ... }`
   for that Secret. The operator wires this onto the data-prepper
   container.
3. Every key in the Secret becomes an environment variable on the
   container.
4. The pipeline references them with `${OS_USER}` and `${OS_PASSWORD}`.
   DataPrepper resolves these against the container environment when it
   loads the pipeline.

## Apply

```sh
# Replace REPLACE_ME with the real password before applying, or use
# kubectl create secret generic ... --from-literal=...
kubectl apply -f secret.yaml
kubectl apply -f cluster.yaml
kubectl apply -f pipeline.yaml
```

## Verify

The credentials are not visible in the pipeline ConfigMap content:

```sh
kubectl -n operator-lab get cm secured-prepper-pipelines -o yaml | grep -E 'OS_USER|OS_PASSWORD'
# username: ${OS_USER}
# password: ${OS_PASSWORD}
```

But they are present as env vars on the running container (admin RBAC
required to see Secret references):

```sh
kubectl -n operator-lab exec deploy/secured-prepper -- env | grep -E '^OS_(USER|PASSWORD)='
```

## Beyond OpenSearch

`spec.envFrom` is just the standard `EnvFromSource` slice. ConfigMaps,
secret prefixes, and selected keys all work. The same pattern fits any
sink or source that supports environment variable substitution
(S3 sink, kafka source, custom processor configs).
