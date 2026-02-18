#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-odh-upgrade}"
KIND_CONFIG="${KIND_CONFIG:-${REPO_ROOT}/components/testing/gh-actions/kind-1-32.yaml}"

require_kind_dependencies() {
  local deps=("kind" "kubectl" "kustomize" "openssl")
  for dep in "${deps[@]}"; do
    if ! command -v "${dep}" >/dev/null 2>&1; then
      echo "Missing required dependency: ${dep}" >&2
      exit 1
    fi
  done
}

kind_cluster_exists() {
  kind get clusters 2>/dev/null | rg -x "${KIND_CLUSTER_NAME}" >/dev/null 2>&1
}

ensure_kind_cluster() {
  require_kind_dependencies

  if kind_cluster_exists; then
    echo "[kind] Reusing existing cluster '${KIND_CLUSTER_NAME}'"
  else
    echo "[kind] Creating cluster '${KIND_CLUSTER_NAME}'"
    kind create cluster --name "${KIND_CLUSTER_NAME}" --config "${KIND_CONFIG}"
  fi
}

load_image_into_kind_if_local() {
  local image_ref="$1"
  if [[ -z "${image_ref}" ]]; then
    return 0
  fi

  # kind can pull remote images directly; local ones must be loaded.
  if [[ "${image_ref}" != localhost/* ]]; then
    return 0
  fi

  local tmp_tar
  tmp_tar="$(mktemp "${TMPDIR:-/tmp}/kind-image-XXXXXX.tar")"
  echo "[kind] Loading local image '${image_ref}'"
  podman save -o "${tmp_tar}" "${image_ref}"
  kind load image-archive --name "${KIND_CLUSTER_NAME}" "${tmp_tar}"
  rm -f "${tmp_tar}"
}

install_kind_prereqs() {
  echo "[kind] Installing Istio"
  "${REPO_ROOT}/components/testing/gh-actions/install_istio.sh"

  echo "[kind] Installing fake OpenShift CRDs"
  kubectl apply -f - <<'EOF'
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    crd/fake: "true"
  name: imagestreams.image.openshift.io
spec:
  group: image.openshift.io
  names:
    kind: ImageStream
    listKind: ImageStreamList
    singular: imagestream
    plural: imagestreams
  scope: Namespaced
  versions:
    - name: v1
      schema:
        openAPIV3Schema:
          type: object
          x-kubernetes-preserve-unknown-fields: true
      served: true
      storage: true
EOF

  echo "[kind] Installing Gateway API CRDs"
  kubectl apply -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.0/standard-install.yaml"
}
