# Workbench 2.x → 3.x Upgrade Scripts

Helper script for migrating RHOAI workbenches from the OAuth-proxy auth model (RHOAI 2.x) to kube-rbac-proxy (RHOAI 3.x).

## Prerequisites

- `oc` CLI logged in with cluster-admin privileges
- `jq` installed

## Usage

```bash
./workbench-2.x-to-3.x-upgrade.sh <command> [--name NAME --namespace NAMESPACE | --all]
```

### Commands

| Command   | Description |
|-----------|-------------|
| `patch`   | Patch notebook CR — removes oauth-proxy sidecar, legacy annotations/finalizers/volumes, strips `--ServerApp.tornado_settings` from `NOTEBOOK_ARGS`, and deletes the StatefulSet. |
| `cleanup` | Remove leftover OAuth related resources (Route, Services, Secrets, OAuthClient). |
| `verify`  | Check that the migration was applied correctly. |

### Targeting

- **Single workbench:** `--name <name> --namespace <namespace>`
- **All workbenches:** `--all`

## Examples

```bash
# Patch a single workbench
./workbench-2.x-to-3.x-upgrade.sh patch --name my-wb --namespace my-ns

# Patch all workbenches in the cluster
./workbench-2.x-to-3.x-upgrade.sh patch --all

# Clean up stale OAuth related resources for all workbenches
./workbench-2.x-to-3.x-upgrade.sh cleanup --all

# Verify migration for a single workbench
./workbench-2.x-to-3.x-upgrade.sh verify --name my-wb --namespace my-ns
```

## Recommended workflow

1. **Patch** — `./workbench-2.x-to-3.x-upgrade.sh patch --all`
2. **Verify** — `./workbench-2.x-to-3.x-upgrade.sh verify --all`
3. **Cleanup** (optional) — `./workbench-2.x-to-3.x-upgrade.sh cleanup --all`

> **Note:**
> During migration (patch), each running workbench is automatically restarted for the changes to take full effect.
> In case of workbenches managed by Kueue, you may have to restart these manually to boot up properly if they were in running state before.
