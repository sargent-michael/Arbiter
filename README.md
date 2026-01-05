# Arbiter

Arbiter is a Kubernetes operator that makes tenant onboarding boring in the best way: declare a tenant once, and Arbiter continuously enforces a secure, repeatable namespace baseline for that tenant.

It watches the `TenantNamespace` custom resource and reconciles the required resources so your platform stays compliant even as drift happens.

---

## What Arbiter Enforces

When `spec.baselinePolicy` is enabled, Arbiter applies a consistent baseline to the tenant namespace:

- Namespace creation and labeling
- Admin RBAC RoleBinding inside the tenant namespace
- ResourceQuota defaults (CPU/memory/pod limits)
- LimitRange defaults (container requests/limits)
- NetworkPolicy defaults:
  - Default deny ingress and egress
  - Allow DNS egress to kube-dns
  - Allow HTTPS ingress on TCP/443

---

## How It Works

1. You create a `TenantNamespace` resource with a `tenantID` and optional settings.
2. Arbiter reconciles the target namespace name (defaults to `tenant-<tenantID>`).
3. Arbiter applies baseline RBAC and resource governance.
4. Arbiter applies network isolation policies (default deny + allow DNS + allow HTTPS ingress).
5. Arbiter reports status and keeps resources aligned over time.

---

## Quick Start (Kind)

```bash
kind create cluster --name arbiter-dev --config kind/kind-cluster.yaml
```

```bash
kind load docker-image ghcr.io/sargent-michael/arbiter:0.0.1 --name arbiter-dev
```

```bash
kubectl apply -k arbiter/config/default
```

```bash
kubectl apply -f samples/sample1.yaml
kubectl apply -f samples/sample2.yaml
```

---

## TenantNamespace CRD

```yaml
apiVersion: platform.upsidedown.io/v1alpha1
kind: TenantNamespace
metadata:
  name: tenant-a
spec:
  tenantID: "ab"
  # targetNamespace: tenant-ab
  adminSubjects:
    - kind: Group
      name: tenant-admins
  baselinePolicy:
    networkIsolation: true
    resourceQuota: true
    limitRange: true
```

### Key Fields

- `spec.tenantID`: Stable tenant identifier (required)
- `spec.targetNamespace`: Explicit namespace name override (optional)
- `spec.adminSubjects`: RBAC subjects granted admin access in the tenant namespace
- `spec.baselinePolicy`: Toggles for baseline enforcement

---

## Image and Deploy

```bash
kubectl apply -k arbiter/config/default
```

To remove:

```bash
kubectl delete -k arbiter/config/default
```

---

## Project Layout

- `arbiter/`: Kubebuilder project and controller code
- `samples/`: example TenantNamespace specs
- `kind/`: local cluster config

---

## License

Apache 2.0. See `LICENSE` for details.
