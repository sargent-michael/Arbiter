#!/usr/bin/env bash
set -euo pipefail

NS="${1:-hellfire-lab}"
USER="${2:-eddie@hawkins.local}"
GROUP="${3:-hellfire-club}"

check() {
  local as_user="$1"
  local as_group="$2"
  local verb="$3"
  local resource="$4"
  if [[ -n "$as_group" ]]; then
    kubectl auth can-i "$verb" "$resource" -n "$NS" --as="$as_user" --as-group="$as_group"
  else
    kubectl auth can-i "$verb" "$resource" -n "$NS" --as="$as_user"
  fi
}

echo "== User subject: $USER in $NS =="
check "$USER" "" get pods
check "$USER" "" create deployments
check "$USER" "" delete pods
check "$USER" "" create rolebindings

echo
echo "== Group subject: $GROUP in $NS =="
# Kubernetes needs a user when impersonating a group, so use a dummy user
check "dummy-user" "$GROUP" get pods
check "dummy-user" "$GROUP" create deployments
check "dummy-user" "$GROUP" delete pods
check "dummy-user" "$GROUP" create rolebindings
