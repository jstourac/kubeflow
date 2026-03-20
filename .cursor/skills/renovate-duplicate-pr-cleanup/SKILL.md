---
name: renovate-duplicate-pr-cleanup
description: >-
  Finds redundant open MintMaker/Konflux or Renovate PRs on
  red-hat-data-services/kubeflow by matching each open PR to merged PRs on the
  same base (first PR often auto-merges; stragglers stay open). Closes only
  high-confidence duplicates with comments linking the canonical merged PR;
  documents GitHub MCP/gh usage, rate limits, and PAT scopes for comment/close.
  Use when the user mentions duplicate bot PRs, stale digest bumps after merge,
  or rhds/kubeflow PR hygiene.
---

# MintMaker / Konflux / Renovate duplicate PR cleanup

## Purpose

Run periodically (or on demand) to clean up **stale bot PRs** when MintMaker/Renovate creates more than one PR for the same bump on a branch.

**Typical pattern on this repo:** the **first** PR **auto-merges** (no conflicts), usually within **1–2 seconds** of creation. Roughly **8–9 minutes later**, MintMaker creates a **second** PR for the identical change on the same branch. This straggler has **automerge explicitly disabled** (its body reads *"Automerge: Disabled because a matching PR was automerged previously"*), so it stays **open** indefinitely even though the update is already on the base branch. Those stragglers are still "duplicates"—compare them against **`is:merged`** PRs, not only against other **`is:open`** PRs.

**Triple-PR edge case:** on some branches, MintMaker creates **two** near-simultaneous PRs that both auto-merge within seconds of each other (likely a race condition in the scheduling pipeline), followed by the straggler ~8–9 minutes later. When searching for merged twins, expect **1 or 2** merged matches per open straggler on the same base branch. Always prefer the **earliest merged** twin as the canonical reference.

**Affected branches:** this pattern occurs on **all** branches that MintMaker targets — not only `rhoai-*` release branches but also **`main`**. Scope the cleanup to every base branch unless the user narrows it.

Triaging **must** include merged PR history per base branch: an open PR with **no** open sibling can still be redundant if a **merged** PR already landed the same change (often earlier by creation/merge time). Close only high-confidence cases and leave an audit trail.

## Defaults (override when the user specifies otherwise)

| Item | Default |
|------|---------|
| Repository | `owner`: `red-hat-data-services`, `repo`: `kubeflow` (short name: rhds/kubeflow) |
| PR source | MintMaker (Renovate) runs on Konflux; PRs are often opened by the org's GitHub App bot, not `renovate[bot]` |
| GitHub access | MCP (`search_pull_requests`, `pull_request_read`, …) **or** `gh` — see rate limits below |

### red-hat-data-services/kubeflow conventions (verify periodically)

- **Bot login** (typical): `app/konflux-internal-p02` — PR author `is_bot: true`.
- **MintMaker head branches**: often `konflux/mintmaker/<base>/<sanitized-image-or-package>`.
- **Titles**: many digest bumps look like `chore(deps): update … docker digest to <sha> (<base>)` — duplicates on the **same base** share the same digest and image path (may differ only in wording if misconfigured).
- **Straggler body signal**: the single strongest indicator of a straggler is the body text *"Automerge: Disabled because a matching PR was automerged previously"*. In practice, **every** straggler observed to date carries this exact phrase. Use it as the primary quick-filter when scanning open PRs before performing the full merged-twin search.

## Preconditions

1. **Read access** to the target repo via GitHub MCP and/or `gh` (list PRs, view diffs, search).
2. **Write access** when performing Step 6 (comment + close): see **Token permissions and failures** below — read-only tokens still work for triage-only runs.
3. User confirms **dry-run vs live close** if not stated: default to **report first**, then close only after user says to proceed (or user explicitly asks to close in the same request).
4. Paginate PR lists (`perPage` / `--limit` + pages) until all candidates in scope are seen.

### GitHub API rate limits

If MCP or REST returns **403** with *secondary rate limit* / *abuse detection*:

1. **Pause** at least **60–120 seconds** before retrying search endpoints.
2. Prefer a **single** `gh pr list` with high `--limit` over many small searches.
3. Avoid tight loops of `get_diff` / `search` — batch PR numbers, fetch diffs only for clusters that are already suspicious.

### Token permissions and failures (comment / close / MCP write)

Triaging (list, search, `pull_request_read`) often succeeds with a **read-only** PAT. **Closing** duplicates requires **write** access on the repository:

| Action | Typical need |
|--------|----------------|
| `gh pr list`, `gh pr view`, `gh pr diff`, `gh search prs` | read |
| `gh pr close --comment`, `add_issue_comment`, `update_pull_request` (`state: closed`) | **Pull requests** (and usually **Contents**) **write** — classic **`repo`** scope or org-approved fine-grained token |

If comment or close fails with **`403 Resource not accessible by personal access token`** (GraphQL or REST), the token is almost certainly **read-only** for that repo. Have the user fix **`gh auth login -h github.com`** (or MCP server credentials) so the identity used for automation matches an account/app with **write** access to `red-hat-data-services/kubeflow` (or the repo in scope). Confirm with **`gh auth status`** and, if needed, test **`gh pr comment`** on a draft.

**Order of operations:** `add_issue_comment` then `update_pull_request` with `"state": "closed"`, or **`gh pr close N --repo … --comment "…"`** which combines both if the token allows.

### `gh` examples

List open bot candidates (JSON):

```bash
gh pr list --repo red-hat-data-services/kubeflow --state open --limit 200 \
  --json number,title,author,baseRefName,headRefName,url,createdAt,updatedAt
```

Merged PR discovery: use the **`--merged`** boolean on **`gh search prs`**. Do **not** use `--state merged` — that flag only accepts `open` or `closed`; merged-only results need **`--merged`**.

```bash
gh search prs "83006d5 base:rhoai-3.4-ea.2" --repo red-hat-data-services/kubeflow --merged --limit 30 \
  --json number,title,url,closedAt,repository
```

## Step 1 — Collect candidate open PRs

1. Scope automation PRs using whichever signals exist (combine if needed):
   - **Search (MCP)**: `search_pull_requests`, e.g. `repo:red-hat-data-services/kubeflow is:pr is:open author:app/konflux-internal-p02` — **and** try `author:renovate[bot]` / `author:app/renovate` for other repos.
   - **List (gh)**: when search is rate-limited, list all open PRs and filter locally by `author.login` / `headRefName` (`konflux/mintmaker/` prefix).
   - **Labels/title**: `chore(deps)`, `fix(deps)`, `Update docker digest`, `Konflux Internal`, etc.
   - **Base branch**: narrow with `base:rhoai-…` in search when supported, or filter after listing.

2. Record for each PR: `number`, `title`, `base`/`baseRefName`, `head`/`headRefName`, author login, URL, timestamps.

3. **Quick open-vs-open clustering** (optional shortcut): group open PRs by **`(base branch, normalized title)`** (normalize by stripping trailing ` (main)` / ` (rhoai-…)` if you use that convention). More than one PR in a group ⇒ candidate duplicate cluster. **Do not stop here** — even a **singleton** open PR still needs **Step 3 merged lookup**.

## Step 2 — Fetch comparable metadata per PR

For each candidate, use `pull_request_read`:

| Method | Use |
|--------|-----|
| `get` | Base/head refs, mergeable state, body (Renovate tables, package names, from→to). |
| `get_files` | Canonical list of changed paths (normalize by path set; paginate). |
| `get_diff` | Whether the PR still has a **non-empty** diff vs base; confirms "already landed" cases. |

Extract from the body/title when possible:

- Package or image name(s) and **version bump** (from → to).
- Renovate **branch / branchName** or **Package** table rows if present.

## Step 3 — Match each open candidate to merged PRs (required)

For **every** open automation PR (not only when several open PRs share a title), decide whether a **merged** PR on the **same base** already landed the same update. Auto-merge usually means that merged PR exists **first**; the open PR is the duplicate.

### 3a — Search merged candidates

For open PR `P` on base `B`, search **`is:merged`** on the same repo, scoped to `B`, with high-signal keywords from `P`:

- **Digest bumps:** search the **short sha** from the title (e.g. `83006d5`) **and** the **image path** fragment (`ubi9/ubi-minimal`, etc.).
- **Version bumps:** package name + new version from title/body.

Use MCP (`search_pull_requests` with `is:merged` and `base:B`) or `gh`, e.g.:

```bash
gh search prs "83006d5 base:rhoai-3.4-ea.2" --repo red-hat-data-services/kubeflow --merged --limit 30 \
  --json number,title,url,closedAt,repository
```

Refine the query if results are noisy. Paginate when needed.

### 3b — Confirm a merged twin (high confidence)

A merged PR `M` is the **canonical** (often auto-merged) sibling of open `P` when **all** hold:

1. **Same base branch** as `P`.
2. **Same logical update** as `P` (title/body agreement, or same digest/version target).
3. **Same change footprint** (compare `get_files` for `M` and `P`; tolerate trivial README-only differences only if the user allows).
4. **Timeline makes sense:** `M` merged **before** `P` was created or updated in the usual case (first lands via auto-merge, second lingers). If timestamps are ambiguous, a match plus **empty `get_diff`** for `P` vs current `B` supports closure — still prefer linking **`M`** in the comment.

If multiple merged PRs match, prefer the **earliest merged** (`closedAt` / merge date) that satisfies the above — that is usually the auto-merged original.

### 3c — Open vs open (still applies)

Two **open** PRs are duplicates when the same conditions as **3b** hold between them (same base, same bump, same files). Resolve them with **one** canonical merged PR when possible; otherwise default to **report only** unless the user wants consolidation.

Optional **strong** signals (not sufficient alone): same bot author, similar `headRefName` prefix, close timestamps.

## Step 4 — Missing merged twin but change on base

If **no** merged PR clearly matches but **`get_diff`** on open `P` is **empty** vs `B` (or GitHub shows no unique commits on the head branch):

1. Treat as **already absorbed** on `B`; hunt the absorbing merge via `list_commits` / `gh api …/commits?sha=B` or merge-train history.
2. Use that **merge SHA** or **merged PR number** in the closing comment. If you can only justify "branch tip already has digest X," **lower confidence** — list under "needs human review" unless the user approves.

If neither a merged PR nor an empty diff supports closure, **skip closing**.

## Step 5 — Choose which open PR(s) to close

- **Merged twin found (`M`):** close **open** `P` (and any other open PRs in the same duplicate group) with a comment pointing to **`M`** — including when **only one** open PR exists ("straggler after auto-merge").
- **Multiple open, no merged match yet:** **do not** auto-close; report only, or apply the keeper heuristic if the user explicitly asked.
- **Keeper heuristic** (user explicitly wants consolidation without a merged twin): keep oldest open PR or best CI — state the rule in the summary.

## Step 6 — Comment, then close

For each PR to close:

1. `add_issue_comment` with a concise explanation and the **merged PR number** (GitHub autolinks `#123` in comments — no separate URL needed):

```markdown
Closing as redundant: this dependency update is already on **`{base}`** (original merge #{merged_number} — typically auto-merged earlier).

- Landed in: #{merged_number}
- Same base branch and equivalent files/change as this PR: `{file_summary}`
```

When you are **not** fully sure, extend the template (for example invite correction or add links) — the default assumes high-confidence closure only.

2. `update_pull_request` with `"state": "closed"` for that PR number.

Order: **comment first**, then close, so notifications stay clear.

## Step 7 — Report back

Give the user a short markdown summary:

- Closed PRs (numbers + links) and the **merged PR** (or commit) cited as the original / auto-merge recipient for each.
- Open PRs that were **singleton stragglers** vs a merged twin vs clusters of multiple open PRs (brief counts).
- PRs examined but not closed (reason: low confidence, no merged counterpart, conflicting file sets).
- Suggested follow-ups (e.g. tune Renovate `branchConcurrentLimit`, `prConcurrentLimit`, or ignore rules) only if the user wants repo config changes.

## Safety rules

- **Do not** close PRs that differ in version target, extra transitive lockfile churn, or additional files unless the user confirms those differences are irrelevant.
- **Do not** bulk-close without triaging each cluster.
- Respect branch protection: if closing fails due to permissions, report and stop rather than retrying destructively.

## Automation hints (optional)

If duplicate PRs are frequent, note for maintainers: align Renovate `branchName` / grouping, reduce concurrent PRs for release branches, or add `prHourlyLimit` / schedule so MintMaker does not recreate the same bump on multiple heads. When the **first** PR auto-merges, duplicate detection must stay **merged-aware** or stragglers accumulate.

### Known limitation: MintMaker configuration changes blocked by CWFHEALTH-4488

As of 2026-03, tuning MintMaker/Renovate configuration (e.g. `prConcurrentLimit`, `branchConcurrentLimit`, scheduling) to prevent duplicate PR creation at the source **cannot be done easily** until [CWFHEALTH-4488](https://redhat.atlassian.net/browse/CWFHEALTH-4488) is implemented on the Konflux/MintMaker side. A workaround was proposed in that Jira issue — check the latest comments there before attempting manual config changes. Until then, periodic cleanup via this skill remains the practical mitigation.
