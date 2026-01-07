# Arbiter Operator

Arbiter is a control plane operator for occupants and namespaces. It watches `Occupant`
resources and enforces a strict, repeatable baseline so every occupant starts clean
and stays compliant.

## What It Reconciles

- Namespace creation + labeling
- Admin RBAC RoleBinding inside occupant namespaces
- Enforcement mode (Permissive or Enforcing) for managed namespaces
- ResourceQuota defaults
- LimitRange defaults
- NetworkPolicy defaults:
  - default deny ingress/egress
  - allow DNS egress to kube-dns
  - allow HTTPS ingress on TCP/443 (overrideable)

## Overrides

Per-occupant overrides can be set in `spec.baselinePolicy`:

- `resourceQuotaSpec` to replace the default ResourceQuota
- `limitRangeSpec` to replace the default LimitRange
- `allowedIngressPorts` to change allowed TCP ingress ports

## Metrics

Prometheus metrics are exposed on the controller manager metrics service and include:

- `arbiter_reconcile_total`
- `arbiter_reconcile_errors_total`
- `arbiter_reconcile_duration_seconds`
- `arbiter_occupant_reconcile_total`
- `arbiter_occupant_reconcile_duration_seconds`
- `arbiter_occupant_namespaces`
- `arbiter_occupant_drift_count`
- `arbiter_occupant_info`
- `arbiter_occupant_enforcement_mode`

The stack chart deploys a minimal Prometheus + Grafana + Loki setup and includes
a ServiceMonitor and Grafana dashboard ConfigMap for Arbiter metrics.

## Local Development

```sh
make test
```

```sh
make run
```

## Build and Deploy

```sh
cd arbiter
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
