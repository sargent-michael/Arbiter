# Arbiter Stack

This chart installs Arbiter and optionally kube-prometheus-stack.

## Install

With Prometheus:

```bash
helm install arbiter-stack dist/chart/arbiter-stack \
  --namespace arbiter-system \
  --create-namespace
```

Without Prometheus:

```bash
helm install arbiter-stack dist/chart/arbiter-stack \
  --namespace arbiter-system \
  --create-namespace \
  --set kube-prometheus-stack.enabled=false
```

## Notes

- The Arbiter webhook requires cert-manager in the cluster. If you already manage
  certs separately, set `arbiter.certManager.enabled=false` and provide your own
  webhook certificate secret name with `arbiter.webhook.certSecretName`.
