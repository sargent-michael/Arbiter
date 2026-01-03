## Overview

Secure Tenant Operator is a Kubernetes Operator that automates the secure, repeatable provisioning of tenant namespaces in a shared Kubernetes cluster.

Instead of manually creating namespaces and then layering security and governance controls by hand, the operator enforces a policy-driven namespace baseline for every tenant. Tenant onboarding becomes declarative, auditable, and continuously enforced through Kubernetes reconciliation.

The operator watches a custom resource (e.g., `TenantNamespace`) that represents a tenant onboarding request and ensures all required baseline resources are created and maintained over time.

---

## Setup and Usage

### Prerequisites

- Kubernetes cluster (Kind for local development or EKS for production)
- `kubectl`
- `docker`
- `kind` (for local development)
- Go toolchain (if building the operator locally)

---

### Local development with Kind

1. **Create a Kind cluster**

   ```bash
   kind create cluster --name platform-dev --config kind/kind-cluster.yaml
   ```
