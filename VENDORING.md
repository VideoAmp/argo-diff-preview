# Vendoring notes

This repo is a VideoAmp vendor of [dag-andersen/argocd-diff-preview](https://github.com/dag-andersen/argocd-diff-preview) (Apache-2.0, LICENSE preserved).

- **Base:** upstream tag `v0.2.16` (the version `tools/ci-gha/scripts/argodiff/install-tools` in VideoAmp/central pins).
- **Patches carried on top:**
  - `fix: include apiVersion in the phase-1 resource matching identity key` — port of [upstream PR #506](https://github.com/dag-andersen/argocd-diff-preview/pull/506) (fixes [#505](https://github.com/dag-andersen/argocd-diff-preview/issues/505)). Pairs resources by `apiVersion/kind/ns/name` instead of `kind/ns/name`; without it, apps carrying same-named Istio + Gloo VirtualServices show phantom mirrored diffs on byte-identical renders.
  - `fix: raise appset generate HTTP timeout to 5 minutes` — the `/api/v1/applicationsets/generate` client timeout was 60s per attempt (5 retries ≈ 305s ceiling). For git generators over the central repo (~630 MB tree) the ephemeral ArgoCD repo-server cold fetch takes 2–5 min on shared runners, so every attempt timed out and killed the whole render (observed as a ~20% flake in `workflow-argocd-diff-preview.yml`, first seen 2026-10-27, always on central-repo-generator appsets).

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
