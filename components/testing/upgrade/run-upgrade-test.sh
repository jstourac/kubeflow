#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

# Defaults intentionally explicit to make baseline reproducible.
MODE="auto"
WORKLOAD_NAMESPACE="${WORKLOAD_NAMESPACE:-upgrade-notebooks}"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-${REPO_ROOT}/components/testing/upgrade/artifacts/$(date +%Y%m%d-%H%M%S)}"
BASELINE_KF_IMAGE="${BASELINE_KF_IMAGE:-quay.io/opendatahub/kubeflow-notebook-controller:main}"
BASELINE_ODH_IMAGE="${BASELINE_ODH_IMAGE:-quay.io/opendatahub/odh-notebook-controller:main}"
TARGET_KF_IMAGE="${TARGET_KF_IMAGE:-}"
TARGET_ODH_IMAGE="${TARGET_ODH_IMAGE:-}"
STRICT_LOGS="${STRICT_LOGS:-false}"
CERT_DIR="${CERT_DIR:-${ARTIFACTS_DIR}/certs}"

usage() {
  cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --mode <auto|kind|openshift>          Cluster mode (default: auto)
  --workload-namespace <name>           Namespace for test notebooks
  --baseline-kf-image <image:tag>       Baseline notebook-controller image
  --baseline-odh-image <image:tag>      Baseline odh-notebook-controller image
  --target-kf-image <image:tag>         Target notebook-controller image
  --target-odh-image <image:tag>        Target odh-notebook-controller image
  --artifacts-dir <path>                Artifact output directory
  --strict-logs <true|false>            Fail on known error patterns in logs
  -h, --help                            Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      MODE="$2"
      shift 2
      ;;
    --workload-namespace)
      WORKLOAD_NAMESPACE="$2"
      shift 2
      ;;
    --baseline-kf-image)
      BASELINE_KF_IMAGE="$2"
      shift 2
      ;;
    --baseline-odh-image)
      BASELINE_ODH_IMAGE="$2"
      shift 2
      ;;
    --target-kf-image)
      TARGET_KF_IMAGE="$2"
      shift 2
      ;;
    --target-odh-image)
      TARGET_ODH_IMAGE="$2"
      shift 2
      ;;
    --artifacts-dir)
      ARTIFACTS_DIR="$2"
      shift 2
      ;;
    --strict-logs)
      STRICT_LOGS="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "${TARGET_KF_IMAGE}" || -z "${TARGET_ODH_IMAGE}" ]]; then
  echo "Both --target-kf-image and --target-odh-image are required." >&2
  exit 1
fi

source "${SCRIPT_DIR}/cluster_kind.sh"
source "${SCRIPT_DIR}/cluster_openshift.sh"
source "${SCRIPT_DIR}/deploy_controllers.sh"
source "${SCRIPT_DIR}/seed-workbenches.sh"
source "${SCRIPT_DIR}/snapshot-state.sh"
source "${SCRIPT_DIR}/assert-invariants.sh"

detect_mode() {
  if [[ "${MODE}" == "kind" || "${MODE}" == "openshift" ]]; then
    echo "${MODE}"
    return 0
  fi

  if command -v oc >/dev/null 2>&1 && oc whoami >/dev/null 2>&1; then
    echo "openshift"
  else
    echo "kind"
  fi
}

prepare_environment() {
  mkdir -p "${ARTIFACTS_DIR}"

  local resolved_mode="$1"
  if [[ "${resolved_mode}" == "kind" ]]; then
    ensure_kind_cluster
    install_kind_prereqs
    load_image_into_kind_if_local "${BASELINE_KF_IMAGE}"
    load_image_into_kind_if_local "${BASELINE_ODH_IMAGE}"
    load_image_into_kind_if_local "${TARGET_KF_IMAGE}"
    load_image_into_kind_if_local "${TARGET_ODH_IMAGE}"
  else
    ensure_logged_in_openshift
    ensure_namespace "opendatahub"
    ensure_namespace "${WORKLOAD_NAMESPACE}"
  fi
}

run_flow() {
  local resolved_mode="$1"
  echo "[upgrade] Mode: ${resolved_mode}"
  echo "[upgrade] Artifacts: ${ARTIFACTS_DIR}"

  echo "[upgrade] Deploying baseline controllers"
  deploy_controllers_with_images "${resolved_mode}" "${BASELINE_KF_IMAGE}" "${BASELINE_ODH_IMAGE}" "${CERT_DIR}/baseline"

  echo "[upgrade] Seeding notebook workloads"
  MODE="${resolved_mode}" WORKLOAD_NAMESPACE="${WORKLOAD_NAMESPACE}" seed_workbenches

  echo "[upgrade] Capturing pre-upgrade snapshot"
  WORKLOAD_NAMESPACE="${WORKLOAD_NAMESPACE}" ARTIFACTS_DIR="${ARTIFACTS_DIR}" SNAPSHOT_NAME="before" \
    NOTEBOOKS_CSV="${RUNNING_NOTEBOOK_NAME},${AUTH_NOTEBOOK_NAME},${STOPPED_NOTEBOOK_NAME}" snapshot_state

  echo "[upgrade] Deploying target controllers"
  deploy_controllers_with_images "${resolved_mode}" "${TARGET_KF_IMAGE}" "${TARGET_ODH_IMAGE}" "${CERT_DIR}/target"

  echo "[upgrade] Capturing post-upgrade snapshot"
  WORKLOAD_NAMESPACE="${WORKLOAD_NAMESPACE}" ARTIFACTS_DIR="${ARTIFACTS_DIR}" SNAPSHOT_NAME="after" \
    NOTEBOOKS_CSV="${RUNNING_NOTEBOOK_NAME},${AUTH_NOTEBOOK_NAME},${STOPPED_NOTEBOOK_NAME}" snapshot_state

  echo "[upgrade] Running invariant checks"
  WORKLOAD_NAMESPACE="${WORKLOAD_NAMESPACE}" ARTIFACTS_DIR="${ARTIFACTS_DIR}" STRICT_LOGS="${STRICT_LOGS}" assert_upgrade_invariants

  echo "[upgrade] SUCCESS"
}

main() {
  local resolved_mode
  resolved_mode="$(detect_mode)"
  prepare_environment "${resolved_mode}"
  run_flow "${resolved_mode}"
}

main
