#!/usr/bin/env bash

set -Eeuo pipefail

WORKLOAD_NAMESPACE="${WORKLOAD_NAMESPACE:-upgrade-notebooks}"
AUTH_NOTEBOOK_NAME="${AUTH_NOTEBOOK_NAME:-upgrade-auth-running}"
RUNNING_NOTEBOOK_NAME="${RUNNING_NOTEBOOK_NAME:-upgrade-running}"
STOPPED_NOTEBOOK_NAME="${STOPPED_NOTEBOOK_NAME:-upgrade-stopped}"
NOTEBOOK_IMAGE="${NOTEBOOK_IMAGE:-quay.io/thoth-station/s2i-minimal-notebook:v0.3.0}"
MODE="${MODE:-kind}"

require_seed_dependencies() {
  local deps=("kubectl")
  for dep in "${deps[@]}"; do
    if ! command -v "${dep}" >/dev/null 2>&1; then
      echo "Missing required dependency: ${dep}" >&2
      exit 1
    fi
  done
}

ensure_workload_namespace() {
  if ! kubectl get namespace "${WORKLOAD_NAMESPACE}" >/dev/null 2>&1; then
    kubectl create namespace "${WORKLOAD_NAMESPACE}"
  fi
}

create_auth_tls_secret_for_kind() {
  if [[ "${MODE}" != "kind" ]]; then
    return 0
  fi

  local cert_dir
  cert_dir="$(mktemp -d "${TMPDIR:-/tmp}/nb-auth-cert-XXXXXX")"
  openssl req -nodes -x509 -newkey rsa:4096 -sha256 -days 365 \
    -keyout "${cert_dir}/tls.key" \
    -out "${cert_dir}/tls.crt" \
    -subj "/CN=${AUTH_NOTEBOOK_NAME}" >/dev/null 2>&1

  kubectl -n "${WORKLOAD_NAMESPACE}" delete secret "${AUTH_NOTEBOOK_NAME}-kube-rbac-proxy-tls" --ignore-not-found=true
  kubectl -n "${WORKLOAD_NAMESPACE}" create secret tls "${AUTH_NOTEBOOK_NAME}-kube-rbac-proxy-tls" \
    --cert="${cert_dir}/tls.crt" \
    --key="${cert_dir}/tls.key"

  rm -rf "${cert_dir}"
}

seed_workbenches() {
  require_seed_dependencies
  ensure_workload_namespace
  create_auth_tls_secret_for_kind

  kubectl apply -f - <<EOF
---
apiVersion: kubeflow.org/v1
kind: Notebook
metadata:
  name: ${RUNNING_NOTEBOOK_NAME}
  namespace: ${WORKLOAD_NAMESPACE}
spec:
  template:
    spec:
      containers:
        - name: ${RUNNING_NOTEBOOK_NAME}
          image: ${NOTEBOOK_IMAGE}
          imagePullPolicy: IfNotPresent
          workingDir: /opt/app-root/src
          env:
            - name: NOTEBOOK_ARGS
              value: |
                --ServerApp.port=8888
                --ServerApp.token=''
                --ServerApp.password=''
                --ServerApp.base_url=/notebook/${WORKLOAD_NAMESPACE}/${RUNNING_NOTEBOOK_NAME}
          ports:
            - name: notebook-port
              containerPort: 8888
              protocol: TCP
          resources:
            requests:
              cpu: "100m"
              memory: "256Mi"
            limits:
              cpu: "500m"
              memory: "1Gi"
---
apiVersion: kubeflow.org/v1
kind: Notebook
metadata:
  name: ${AUTH_NOTEBOOK_NAME}
  namespace: ${WORKLOAD_NAMESPACE}
  annotations:
    notebooks.opendatahub.io/inject-auth: "true"
spec:
  template:
    spec:
      containers:
        - name: ${AUTH_NOTEBOOK_NAME}
          image: ${NOTEBOOK_IMAGE}
          imagePullPolicy: IfNotPresent
          workingDir: /opt/app-root/src
          env:
            - name: NOTEBOOK_ARGS
              value: |
                --ServerApp.port=8888
                --ServerApp.token=''
                --ServerApp.password=''
                --ServerApp.base_url=/notebook/${WORKLOAD_NAMESPACE}/${AUTH_NOTEBOOK_NAME}
          ports:
            - name: notebook-port
              containerPort: 8888
              protocol: TCP
          resources:
            requests:
              cpu: "100m"
              memory: "256Mi"
            limits:
              cpu: "500m"
              memory: "1Gi"
---
apiVersion: kubeflow.org/v1
kind: Notebook
metadata:
  name: ${STOPPED_NOTEBOOK_NAME}
  namespace: ${WORKLOAD_NAMESPACE}
  annotations:
    kubeflow-resource-stopped: "true"
spec:
  template:
    spec:
      containers:
        - name: ${STOPPED_NOTEBOOK_NAME}
          image: ${NOTEBOOK_IMAGE}
          imagePullPolicy: IfNotPresent
          workingDir: /opt/app-root/src
          env:
            - name: NOTEBOOK_ARGS
              value: |
                --ServerApp.port=8888
                --ServerApp.token=''
                --ServerApp.password=''
                --ServerApp.base_url=/notebook/${WORKLOAD_NAMESPACE}/${STOPPED_NOTEBOOK_NAME}
          ports:
            - name: notebook-port
              containerPort: 8888
              protocol: TCP
          resources:
            requests:
              cpu: "100m"
              memory: "256Mi"
            limits:
              cpu: "500m"
              memory: "1Gi"
EOF

  kubectl -n "${WORKLOAD_NAMESPACE}" rollout status "statefulset/${RUNNING_NOTEBOOK_NAME}" --timeout=180s
  kubectl -n "${WORKLOAD_NAMESPACE}" rollout status "statefulset/${AUTH_NOTEBOOK_NAME}" --timeout=180s
  kubectl -n "${WORKLOAD_NAMESPACE}" wait pod "${RUNNING_NOTEBOOK_NAME}-0" --for=condition=Ready --timeout=180s
  kubectl -n "${WORKLOAD_NAMESPACE}" wait pod "${AUTH_NOTEBOOK_NAME}-0" --for=condition=Ready --timeout=180s
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  seed_workbenches
fi
