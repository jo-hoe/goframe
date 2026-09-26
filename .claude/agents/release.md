---
name: release
description: Release a new version of goframe. Supports two release types — full (new image + charts) and chart-only (chart bump, existing image). Run in foreground (run_in_background: false) so step progress is visible in real time.
allowedTools:
  - Read
  - Edit
  - Bash(git *)
  - Bash(gh *)
  - Bash(helm *)
  - Bash(grep *)
  - Bash(make *)
---

## Release process for goframe

Follow these steps in order. Do not skip steps. After each step, report completion with a one-line status so the user can track progress.

### Step 1 — Determine version and release type

Report: `[Step 1/5] Determining version and release type...`

Check the current versions and what has changed since the last tag:
```bash
grep -E '^version:|^appVersion:' charts/goframe/Chart.yaml
grep -E '^version:|^appVersion:' charts/goframe-operator/Chart.yaml
git tag --sort=-v:refname | head -3
git log $(git tag --sort=-v:refname | head -1)..HEAD --oneline
git diff $(git tag --sort=-v:refname | head -1)..HEAD -- charts/ *.go cmd/ internal/ go.mod go.sum Dockerfile
```

**Determine release type from the diff:**
- **No release needed** — only non-app, non-chart files changed (e.g. `.claude/`, `docs/`, `Makefile`, CI config). Commit + push any uncommitted working-tree changes and stop. Report: `[Step 1/5] No version bump needed — committing non-release changes and pushing.`
- **Full release** — app code changed (`cmd/`, `internal/`, `go.mod`, `go.sum`, `Dockerfile`). Commits app changes, pushes a semver tag (triggers Docker image build CI), then bumps both `version` and `appVersion` in both charts and pushes the chart bump commit (triggers chart release CI).
- **Chart-only release** — only chart templates/values changed (`charts/`), no app code changes. Bumps only `version` in both charts, keeps `appVersion`. No new tag pushed.

Report: `[Step 1/5] ✓ Release type: <full|chart-only>, new version: <new-version>, appVersion: <app-version>`

### Step 2 — Commit app changes and (for full releases) tag

Report: `[Step 2/5] Committing app changes and tagging...`

**Check for uncommitted changes and commit them if present:**
```bash
git status --short
```

If there are any modified or untracked files (app code, tests, config, docs), stage and commit them:
```bash
git add <each modified or untracked file>
git commit -m "feat: <short summary of the changes>"
git push origin main
```

Use `git diff --cached` and `git log --oneline -5` to write an accurate commit message.

**Full release only** — push the semver tag immediately after the app commit to trigger the Docker image build CI. The tag must be pushed *before* the chart bump so the image build is already running while charts are being updated:
```bash
git tag v<new-version>
git push origin v<new-version>
```

If push fails due to remote changes, rebase first:
```bash
git fetch origin && git rebase origin/main
```
Then re-push (and re-tag if needed).

Report: `[Step 2/5] ✓ App changes pushed` (and `+ tag v<new-version>` for full releases)

### Step 3 — Regenerate Helm docs and bump Chart.yaml

Report: `[Step 3/5] Regenerating Helm docs and bumping Chart.yaml...`

**First, regenerate Helm chart documentation** (required before every release):
```bash
make generate-helm-docs
```

This updates `charts/goframe/README.md` and `charts/goframe-operator/README.md`.

**Then bump versions in both `charts/goframe/Chart.yaml` and `charts/goframe-operator/Chart.yaml`:**

**Full release:** update both fields in each Chart.yaml:
```yaml
version: <new-version>
appVersion: "<new-version>"
```

**Chart-only release:** update only `version` in each Chart.yaml, leave `appVersion` unchanged:
```yaml
version: <new-version>
appVersion: "<current-app-version>"   # unchanged
```

Commit and push the Helm docs + chart version bump together:
```bash
git add charts/
git commit -m "chore: bump chart versions to <new-version>"
git push origin main
```

Report: `[Step 3/5] ✓ Helm docs regenerated, Chart.yaml files updated and pushed`

### Step 4 — Babysit CI

Report: `[Step 4/5] Waiting for CI (timeout: 10 minutes)...`

Poll every 30 seconds, up to 20 times. On each poll:
```bash
gh run list --repo jo-hoe/goframe --limit 10
```

**Full release** — track these workflows:
- `lint` — triggered by the main push
- `test` — triggered by the main push
- `Release Image` — triggered by the `v<new-version>` tag (builds and pushes the Docker image)
- `Release Charts` — triggered by the `charts/goframe/Chart.yaml` change on main

**Chart-only release** — track only:
- `lint` — triggered by the main push
- `test` — triggered by the main push
- `Release Charts` — triggered by the `charts/goframe/Chart.yaml` change on main

Report each poll as: `[Step 4/5] Poll <n>/20 — lint: <status>, test: <status>, image: <status|n/a>, charts: <status>`

Stop as soon as all tracked workflows show `completed`. If any shows `failure`, fetch logs immediately:
```bash
gh run view <id> --log-failed
```
Then report the failure and stop.

Note: `Release Charts` publishes to the `gh-pages` branch via `chart-releaser`, which then triggers `pages-build-deployment`. The chart is not live on the Helm repo until that Pages deployment completes — allow for it in Step 5.

If 20 polls pass without completion: `[Step 4/5] ✗ Timeout after 10 minutes` and stop.

Report: `[Step 4/5] ✓ All workflows completed successfully`

### Step 5 — Verify and confirm

Report: `[Step 5/5] Verifying published artifacts...`

Verify the chart was published as a GitHub release by `chart-releaser`:
```bash
gh release view goframe-<new-version> --repo jo-hoe/goframe
```

Then confirm it is served from the Pages Helm repo (may lag until the Pages deployment from Step 4 finishes — retry a few times if needed):
```bash
helm repo add goframe https://jo-hoe.github.io/goframe 2>/dev/null; helm repo update goframe
helm search repo goframe/goframe --version <new-version>
```

For full releases, the image tag is confirmed by the `Release Image` workflow succeeding in Step 4.

If chart verification fails, report the error and stop.

Report: `[Step 5/5] ✓ Release complete`

Confirm:
- Chart release: `goframe-<new-version>` on jo-hoe/goframe
- Helm repo: `https://jo-hoe.github.io/goframe` → `goframe/goframe --version <new-version>`
- Image: `ghcr.io/jo-hoe/goframe:v<app-version>` (full release) or `unchanged at v<current-app-version>` (chart-only)
