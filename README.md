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
helm repo add arbiter https://sargent-michael.github.io/Arbiter/packages
helm repo update
helm install arbiter-stack arbiter/arbiter-stack \
  --namespace arbiter-system \
  --create-namespace \
  --set arbiter.image.tag=0.0.6 \
  --set kube-prometheus-stack.enabled=true
kubectl rollout status deploy/arbiter-stack-controller-manager -n arbiter-system
kubectl apply -f samples/sample1.yaml
kubectl apply -f samples/sample2.yaml
```

The stack chart installs cert-manager, Prometheus, and Arbiter in one shot.

---

## TenantNamespace CRD

```yaml
apiVersion: arbiter.io/v1alpha1
kind: TenantNamespace
metadata:
  name: hawkins
spec:
  tenantID: "hawkins"
  # targetNamespace: hawkins-lab
  adminSubjects:
    - kind: Group
      name: hellfire-club
  baselinePolicy:
    networkIsolation: true
    resourceQuota: true
    limitRange: true
    # allowedIngressPorts: [443, 8443]
    # resourceQuotaSpec:
    #   hard:
    #     requests.cpu: "4"
    #     requests.memory: 8Gi
    # limitRangeSpec:
    #   limits:
    #     - type: Container
    #       defaultRequest:
    #         cpu: 250m
    #         memory: 256Mi
    #       default:
    #         cpu: "1"
    #         memory: 1Gi
```

### Key Fields

- `spec.tenantID`: Stable tenant identifier (required)
- `spec.targetNamespace`: Explicit namespace name override (optional)
- `spec.adminSubjects`: RBAC subjects granted admin access in the tenant namespace
- `spec.baselinePolicy`: Toggles for baseline enforcement
- `spec.baselinePolicy.resourceQuotaSpec`: Override default ResourceQuota spec
- `spec.baselinePolicy.limitRangeSpec`: Override default LimitRange spec
- `spec.baselinePolicy.allowedIngressPorts`: Override allowed ingress TCP ports

---

## Apply Overrides

Example with overrides:

```bash
kubectl apply -f samples/sample2.yaml
```

Or inline:

```yaml
apiVersion: arbiter.io/v1alpha1
kind: TenantNamespace
metadata:
  name: starcourt
spec:
  tenantID: "starcourt"
  baselinePolicy:
    allowedIngressPorts: [443, 8443]
    resourceQuotaSpec:
      hard:
        requests.cpu: "3"
        requests.memory: 6Gi
        limits.cpu: "6"
        limits.memory: 12Gi
        pods: "75"
    limitRangeSpec:
      limits:
        - type: Container
          defaultRequest:
            cpu: 200m
            memory: 256Mi
          default:
            cpu: "1"
            memory: 1Gi
```

---

## Metrics

Arbiter exposes Prometheus metrics on the controller manager metrics service.

```bash
kubectl get svc -n arbiter-system arbiter-controller-manager-metrics-service
```

Key metrics:

- `arbiter_reconcile_total` (labels: `controller`, `result`)
- `arbiter_reconcile_errors_total` (label: `controller`)
- `arbiter_reconcile_duration_seconds` (labels: `controller`, `result`)

Arbiter ships a ServiceMonitor and a ClusterRoleBinding when installed via Helm.
Verify targets:

```bash
kubectl port-forward -n kube-prometheus-stack svc/kube-prometheus-stack-prometheus 9090:9090
```

Open `http://localhost:9090/targets` and confirm the Arbiter target is UP.

---

## Grafana Dashboard

The Helm stack installs the Arbiter dashboard automatically. To view it:

```bash
kubectl port-forward -n arbiter-system svc/arbiter-stack-grafana 3000:80
```

Then open `http://localhost:3000` and look for “Arbiter - Reconciliation”.

---

## CLI Shortcuts

The TenantNamespace CRD includes a short name, so you can use:

```bash
kubectl get tns
kubectl get tns <tenant>
kubectl delete tns <tenant>
```

To tail Arbiter logs with `kubectl arbiter-logs`, add the plugin script to your PATH:

```bash
export PATH="$PATH:$(pwd)/scripts"
```

Then run:

```bash
kubectl arbiter-logs
kubectl arbiter-logs -n arbiter-system --tail=100
```

---

## Helm Install

```bash
helm repo add arbiter https://sargent-michael.github.io/Arbiter/packages
helm repo update
helm upgrade --install arbiter-stack arbiter/arbiter-stack \
  --namespace arbiter-system \
  --create-namespace \
  --set arbiter.image.tag=0.0.6
```

To remove:

```bash
helm uninstall arbiter-stack -n arbiter-system
```

---

## License

Apache 2.0. See `LICENSE` for details.
