# Arbiter

```
    ___         __     _ __           
   /   |  _____/ /_   (_) /____  _____
  / /| | / ___/ __ \ / / __/ _ \/ ___/
 / ___ |/ /  / /_/ // / /_/  __/ /    
/_/  |_/_/  /_.___//_/\__/\___/_/ v1.0.0    
-----------------------------------------
```

Arbiter is a Kubernetes operator that turns occupant onboarding into a single, repeatable declaration.
You define an `Occupant`, and Arbiter enforces namespaces, RBAC, quotas, limits, and network policy
with continuous reconciliation.

---

## Highlights

- Multi-namespace occupants with deterministic naming (`occupant-<occupantID>` by default)
- Global Baseline with per-occupant overrides for quotas, limits, and network policy
- Status summaries in `kubectl get occupants` for fast validation
- Managed namespaces can be `Enforcing` or `Permissive`
- Helm stack installs cert-manager, Prometheus, Grafana, Loki, and Arbiter together
- Metrics and dashboards included out of the box
- Per-occupant metrics with a dedicated Grafana dashboard

---

## Quickstart

<div>
<details open>
<summary>Helm (Kind)</summary>

```bash
kind create cluster --name arbiter-dev --config kind/kind-cluster.yaml
helm repo add arbiter https://sargent-michael.github.io/Arbiter
helm repo update
helm upgrade --install arbiter-stack arbiter/arbiter-stack \
  --namespace arbiter-system \
  --create-namespace \
  --set arbiter.image.tag=1.0.10 \
  --version 0.1.16
kubectl rollout status deploy/arbiter-stack-controller-manager -n arbiter-system
```

Then apply samples:

```bash
kubectl apply -f samples/baseline.yaml
kubectl apply -f samples/sample1.yaml
kubectl apply -f samples/sample2.yaml
kubectl apply -f samples/sample3.yaml
kubectl apply -f samples/sample4.yaml
```

</details>

<details>
<summary>Helm (Existing Cluster)</summary>

```bash
helm repo add arbiter https://sargent-michael.github.io/Arbiter
helm repo update
helm upgrade --install arbiter-stack arbiter/arbiter-stack \
  --namespace arbiter-system \
  --create-namespace \
  --set arbiter.image.tag=1.0.10 \
  --version 0.1.16

kubectl rollout status deploy/arbiter-stack-controller-manager -n arbiter-system
```

</details>
</div>

---
## Upgrades (Existing CRDs)

If the CRDs already exist (from a previous install), use `--skip-crds`:

```bash
helm upgrade --install arbiter-stack arbiter/arbiter-stack \
  --namespace arbiter-system \
  --create-namespace \
  --set arbiter.image.tag=1.0.10 \
  --version 0.1.16 \
  --skip-crds
```

---

## Occupant CRD

```yaml
apiVersion: project-arbiter.io/v1alpha1
kind: Occupant
metadata:
  name: hawkins
spec:
  occupantID: "hawkins"
  # targetNamespace: occupant-hawkins
  # targetNamespaces:
  #   - hawkins-lab
  #   - hawkins-ops
  # adoptedNamespaces:
  #   - hawkins-legacy
  # enforcementMode: Permissive
  # capabilities: ["rbac", "net", "obs", "ci"]
  adminSubjects:
    - kind: Group
      name: hellfire-club
  externalIntegrations:
    ci: github-actions
    observability: grafana
  occupantPolicy:
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

- `spec.occupantID`: Stable occupant identifier (required)
- `spec.targetNamespace`: Explicit namespace override (optional)
- `spec.targetNamespaces`: List of namespaces managed for the occupant (optional)
- `spec.adoptedNamespaces`: Existing namespaces to adopt without deletion lifecycle (optional)
- `spec.adminSubjects`: RBAC subjects granted admin access
- `spec.enforcementMode`: `Enforcing` or `Permissive` policy enforcement (optional)
- `spec.capabilities`: Declared capability labels for status summaries (optional)
- `spec.externalIntegrations`: CI/observability/cost integration labels (optional)
- `spec.occupantPolicy`: Toggle and override baseline enforcement (if omitted, Baseline defaults apply)

---

## Baseline Defaults

```yaml
apiVersion: project-arbiter.io/v1alpha1
kind: Baseline
metadata:
  name: default
spec:
  baselinePolicy:
    networkIsolation: true
    resourceQuota: true
    limitRange: true
    allowedIngressPorts: [443]
```

---

## Samples (Stranger Things)

- `samples/sample1.yaml`: simple Occupant with defaults (Starcourt)
- `samples/sample2.yaml`: overrides + extra RoleBindings (Upside Down)
- `samples/sample3.yaml`: multi-namespace occupant (Hawkins R&D)
- `samples/sample4.yaml`: multi-namespace + multiple ingress ports

---

## Observability

Metrics are exposed on the controller manager metrics service.

```bash
kubectl get svc -n arbiter-system arbiter-stack-controller-manager-metrics-service
```

Grafana dashboards and Loki datasources are installed by the stack chart:

```bash
kubectl port-forward -n arbiter-system svc/arbiter-stack-grafana 3000:80
```

Prometheus and Loki are exposed inside the cluster; Grafana is prewired to both.

---

## CLI Shortcuts

```bash
kubectl get occupants
kubectl get occ
kubectl get project
kubectl get project <occupant>
kubectl delete occ <occupant>
```

Example `kubectl get occupants` output:

```
NAME          NAMESPACES        ENFORCEMENT   CAPABILITIES        HEALTH   RECONCILES
hellfire      3 (managed)       Enforcing     rbac,net,obs,ci     OK       12
starcourt     1 (adopted)       Permissive    rbac,obs            WARN     4
```

`kubectl describe project <project-id>` now includes:

- Identity bindings (groups/users)
- Enabled capabilities
- Enforcement status (Enforcing/Permissive)
- Drift summary
- External integrations (CI, observability, cost)
- Last reconciliation outcome

Arbiter logs via kubectl plugin:

```bash
export PATH="$PATH:$(pwd)/scripts"
kubectl arbiter-logs -n arbiter-system --tail=200
```

---

## DevSecOps Pipeline

This repo ships with automated security checks:

- CodeQL (code scanning)
- Govulncheck (Go vulnerability DB)
- Gosec (Go SAST)
- Trivy (filesystem + image scanning)
- Dependency Review (PR guardrails)

---

## Uninstall

```bash
helm uninstall arbiter-stack -n arbiter-system
```
