# Arbiter Operator

Arbiter is a control plane operator for settlers and namespaces. It watches `Settler`
resources and enforces a strict, repeatable baseline so every settler starts clean
and stays compliant.

## What It Reconciles

- Namespace creation + labeling
- Admin RBAC RoleBinding inside settler namespaces
- ResourceQuota defaults
- LimitRange defaults
- NetworkPolicy defaults:
  - default deny ingress/egress
  - allow DNS egress to kube-dns
  - allow HTTPS ingress on TCP/443 (overrideable)

## Overrides

Per-settler overrides can be set in `spec.baselinePolicy`:

- `resourceQuotaSpec` to replace the default ResourceQuota
- `limitRangeSpec` to replace the default LimitRange
- `allowedIngressPorts` to change allowed TCP ingress ports

## Metrics

Prometheus metrics are exposed on the controller manager metrics service and include:

- `arbiter_reconcile_total`
- `arbiter_reconcile_errors_total`
- `arbiter_reconcile_duration_seconds`

For kube-prometheus-stack, the default config includes a ServiceMonitor and a
ClusterRoleBinding for the Prometheus service account.

## Local Development

```sh
make test
```

```sh
make run
```

## Build and Deploy

```sh
make docker-build docker-push IMG=<some-registry>/arbiter:tag
```

```sh
make install
make deploy IMG=<some-registry>/arbiter:tag
```

## Uninstall

```sh
make undeploy
make uninstall
```

## Samples

```sh
kubectl apply -k config/samples/
```

## License

Apache 2.0.
