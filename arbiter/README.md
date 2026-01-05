# Arbiter Operator

Arbiter is the control plane bouncer for tenant namespaces. It watches `TenantNamespace` resources and enforces a strict, repeatable baseline so every tenant starts clean and stays compliant.

## What It Reconciles

- Namespace creation + labeling
- Admin RBAC RoleBinding inside the tenant namespace
- ResourceQuota defaults
- LimitRange defaults
- NetworkPolicy defaults:
  - default deny ingress/egress
  - allow DNS egress to kube-dns
  - allow HTTPS ingress on TCP/443

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
