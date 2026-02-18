# Notebook Controller Upgrade Test

This directory contains a lightweight, iterative upgrade test for:

- `components/notebook-controller`
- `components/odh-notebook-controller`

The goal is to validate that upgrading both controllers from a baseline image to
the current target images does not break existing workbench workloads.

## What This Covers

The test performs:

1. cluster mode selection (`kind`, `openshift`, or `auto`)
2. baseline controller deployment
3. workload seeding (running notebook, auth-injected running notebook, stopped notebook)
4. pre-upgrade snapshot capture
5. target controller rollout
6. post-upgrade snapshot capture
7. machine-checkable invariants:
   - running notebook pods are not recreated (UID/start time unchanged)
   - running notebook pods do not increase restart count
   - stopped notebook remains stopped
   - expected generated resources exist (StatefulSet, Service, HTTPRoute, NetworkPolicies)
   - controllers are healthy after rollout
   - optional strict log pattern checks

## What This Does Not Cover

- Full RHOAI operator lifecycle and OLM bundle behavior.
- Full product integration with every dependent controller/service.
- Multi-version upgrade matrix (MVP uses one baseline at a time).
- Rollback validation (target to baseline).

## Files

- `run-upgrade-test.sh`: orchestrates baseline -> seed -> snapshot -> upgrade -> assert.
- `cluster_kind.sh`: KinD provisioning/prerequisites (Istio, fake OpenShift CRD, Gateway API).
- `cluster_openshift.sh`: validates an existing `oc` login/session.
- `deploy_controllers.sh`: deploy helpers for baseline/target images.
- `seed-workbenches.sh`: reusable workload seeding script.
- `snapshot-state.sh`: deterministic pre/post snapshots and logs/events capture.
- `assert-invariants.sh`: upgrade assertions.

## Local Usage

Prerequisites:

- `kubectl`, `kustomize`, `kind`, `podman`, `openssl`, `rg`
- for OpenShift mode: `oc` and logged-in cluster

Build target images from current checkout:

```sh
cd components/notebook-controller
make docker-build IMG=localhost/notebook-controller TAG=upgrade-test

cd ../odh-notebook-controller
make docker-build IMG=localhost/odh-notebook-controller TAG=upgrade-test
```

Run upgrade test in KinD mode:

```sh
bash components/testing/upgrade/run-upgrade-test.sh \
  --mode kind \
  --baseline-kf-image quay.io/opendatahub/kubeflow-notebook-controller:main \
  --baseline-odh-image quay.io/opendatahub/odh-notebook-controller:main \
  --target-kf-image localhost/notebook-controller:upgrade-test \
  --target-odh-image localhost/odh-notebook-controller:upgrade-test \
  --strict-logs false
```

Run upgrade test against already logged-in OpenShift:

```sh
bash components/testing/upgrade/run-upgrade-test.sh \
  --mode openshift \
  --baseline-kf-image quay.io/opendatahub/kubeflow-notebook-controller:main \
  --baseline-odh-image quay.io/opendatahub/odh-notebook-controller:main \
  --target-kf-image localhost/notebook-controller:upgrade-test \
  --target-odh-image localhost/odh-notebook-controller:upgrade-test
```

Artifacts are written to:

- `components/testing/upgrade/artifacts/<timestamp>/`

## CI Workflow

Workflow file:

- `.github/workflows/odh_notebook_controller_upgrade_test.yaml`

Behavior:

- builds target controller images with podman
- runs KinD-based upgrade test
- uploads snapshots/logs/events as artifacts
- supports `workflow_dispatch` baseline image overrides

## Extending Later

- Add baseline matrix (N-1, N-2) in a scheduled workflow.
- Add workload profiles (custom RBAC, additional notebook images, kueue profiles).
- Add optional rollback checks as non-blocking jobs first.
