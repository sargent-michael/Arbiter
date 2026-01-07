# Arbiter Stack

This chart installs Arbiter with a minimal Prometheus + Grafana + Loki stack.

## Install

```bash
helm install arbiter-stack dist/chart/arbiter-stack \
  --namespace arbiter-system \
  --create-namespace
```

## Notes

- The Arbiter webhook requires cert-manager in the cluster. If you already manage
  certs separately, set `arbiter.certManager.enabled=false` and provide your own
  webhook certificate secret name with `arbiter.webhook.certSecretName`.
- Grafana datasources are preconfigured for Prometheus and Loki.
- Arbiter logs are available in Loki under the `arbiter-system` namespace labels.
