# Vendoring notes

This repo is a VideoAmp vendor of [dag-andersen/argocd-diff-preview](https://github.com/dag-andersen/argocd-diff-preview) (Apache-2.0, LICENSE preserved).

- **Base:** upstream tag `v0.2.16` (the version `tools/ci-gha/scripts/argodiff/install-tools` in VideoAmp/central pins).
- **Patches carried on top:**
  - `fix: include apiVersion in the phase-1 resource matching identity key` — port of [upstream PR #506](https://github.com/dag-andersen/argocd-diff-preview/pull/506) (fixes [#505](https://github.com/dag-andersen/argocd-diff-preview/issues/505)). Pairs resources by `apiVersion/kind/ns/name` instead of `kind/ns/name`; without it, apps carrying same-named Istio + Gloo VirtualServices show phantom mirrored diffs on byte-identical renders.
  - `fix: raise appset generate HTTP timeout to 5 minutes` — the `/api/v1/applicationsets/generate` client timeout was 60s per attempt (5 retries ≈ 305s ceiling). For git generators over the central repo (~630 MB tree) the ephemeral ArgoCD repo-server cold fetch takes 2–5 min on shared runners, so every attempt timed out and killed the whole render (observed as a ~20% flake in `workflow-argocd-diff-preview.yml`, first seen 2026-10-27, always on central-repo-generator appsets).
  - `feat: --skip-failed-apps render tolerance` — upstream discards every successful extraction when any single application fails to render, so one broken app (e.g. a stale argocd-apps-params entry whose values file is gone at HEAD) costs the PR its entire diff preview. Tolerant mode keeps every other application in the diff, logs each skipped app as `⏭️ Skipping application <name> [<b|t>|<appset>]: <err>` (the shape `report-render-failures` in central parses) plus a `🏁 Tolerant render completed: skipped=N rendered_base=N rendered_target=N` completion line, and drops a failed app from BOTH branches (all-or-nothing) so a target-side failure cannot diff as "deleted by this PR". VideoAmp-only need; no upstream PR.
  - `fix: raise manifests HTTP client timeout above the api-server repo-server deadline` — the `/api/v1/applications/<app>/manifests` client timeout was 30s. During fleet renders the repo-server's per-repo `repositoryLock` serializes manifest generations, and the api-server holds each GET up to its repo-server RPC deadline (900s once VideoAmp's values-override applies). The 30s cut killed requests that were merely queued: the caller then refreshed the app (requeueing another generation behind the same lock) and retried, amplifying the queue — observed as 99-106 `Client.Timeout exceeded while awaiting headers` cycles per stalled fleet render. 960s sits just above the 900s server deadline so slow-but-progressing generations complete on the first try.
  - `fix: fast-abort on set-app-instance tracking-info failures` — an ApplicationSet templating a nil metadata value (observed: a cluster registry label without its `*_iam_role_arn` annotation rendering `eks.amazonaws.com/role-arn: null`) fails ArgoCD manifest generation with `failed to set app instance tracking info ... contains non-string value ...` — deterministic, but absent from the fast-abort `errorMessages` list, so each such app burned one of the extraction pool's concurrency slots polling until the per-app timeout (71 such pairs suppressed fleet throughput ~3x). Added to `errorMessages`: the app fails in seconds and `--skip-failed-apps` drops it from the report instead of poisoning the pool.
  - `feat: --failures-report structured render report` — the per-application failures the tolerant render already collected were only emitted as log lines that consumers regex-parse. `--failures-report PATH` writes a deterministic JSON report after the render (budget/tolerance header plus one record per failed application: name, branch, ApplicationSet file, failure phase `never-started`/`timeout`/`error` classified from the canonical error prefixes, full error text, sorted by branch/appset/name) so wrapper tooling never has to parse the log. Server-api render method only; the flag refuses the other extraction paths. VideoAmp-only need; no upstream PR.

## Pulling upstream

```bash
git remote add upstream https://github.com/dag-andersen/argocd-diff-preview.git  # if missing
git fetch upstream --tags
git checkout -b va-<new-base> v<new-base-tag>
git rebase --onto va-<new-base> <previous-va-tag-or-base> main   # replay carried patches
# resolve, run: go build ./... && go test ./pkg/... ./cmd/...
```

## Tagging / releases

Releases follow `v<upstream-version>-va.N` (e.g. `v0.2.16-va.1`) and publish the same asset layout upstream uses:

```
argocd-diff-preview-Linux-x86_64.tar.gz   # tarball root contains the single `argocd-diff-preview` binary
```

Build: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o argocd-diff-preview .`

`install-tools` in central verifies the tarball SHA256 against the release asset digest.
