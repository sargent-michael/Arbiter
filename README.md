# Arbiter

```
    ___         __     _ __           
   /   |  _____/ /_   (_) /____  _____
  / /| | / ___/ __ \ / / __/ _ \/ ___/
 / ___ |/ /  / /_/ // / /_/  __/ /    
/_/  |_/_/  /_.___//_/\__/\___/_/ v1.0.0    
-----------------------------------------
```

Arbiter is a Kubernetes operator that turns project onboarding into a single, repeatable declaration.
You define a `Project`, and Arbiter enforces namespaces, RBAC, quotas, limits, and network policy
with continuous reconciliation.

---

## Highlights

- Multi-namespace projects with deterministic naming (`project-<projectID>` by default)
- Global Baseline with per-project overrides for quotas, limits, and network policy
- Status summaries in `kubectl get proj` for fast validation
- Helm stack installs cert-manager, Prometheus, Grafana, and Arbiter together
- Metrics and dashboards included out of the box

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
  --set arbiter.image.tag=<version> \
  --set kube-prometheus-stack.enabled=true
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
  --set arbiter.image.tag=<version>

kubectl rollout status deploy/arbiter-stack-controller-manager -n arbiter-system
```

</details>
</div>

---

## Project CRD

```yaml
apiVersion: project-arbiter.io/v1alpha1
kind: Project
metadata:
  name: hawkins
spec:
  projectID: "hawkins"
  # targetNamespace: project-hawkins
  # targetNamespaces:
  #   - hawkins-lab
  #   - hawkins-ops
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

- `spec.projectID`: Stable project identifier (required)
- `spec.targetNamespace`: Explicit namespace override (optional)
- `spec.targetNamespaces`: List of namespaces managed for the project (optional)
- `spec.adminSubjects`: RBAC subjects granted admin access
- `spec.baselinePolicy`: Toggle and override baseline enforcement

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

- `samples/sample1.yaml`: simple Project with defaults (Starcourt)
- `samples/sample2.yaml`: overrides + extra RoleBindings (Upside Down)
- `samples/sample3.yaml`: multi-namespace project (Hawkins R&D)
- `samples/sample4.yaml`: multi-namespace + multiple ingress ports

---

## Observability

Metrics are exposed on the controller manager metrics service.

```bash
kubectl get svc -n arbiter-system arbiter-stack-controller-manager-metrics-service
```

Grafana dashboard is installed by the stack chart:

```bash
kubectl port-forward -n arbiter-system svc/arbiter-stack-grafana 3000:80
```

---

## CLI Shortcuts

```bash
kubectl get projects
kubectl get proj
kubectl get proj <project>
kubectl delete proj <project>
```

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
