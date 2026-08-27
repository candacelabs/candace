# Destination CI

These workflows run in **`candacelabs/candace`**, the published repository this
tree is exported to. They ship with the snapshot: the export declaration in the
monorepo's `Candacefile` sets `requires_workflows_write: true` precisely so the
publisher may write this directory.

## They do not run where they are written

In the monorepo this file lives at `candace/.github/workflows/`, and GitHub only
reads workflows from a repository's own root `.github/workflows/`. So these are
inert there — by construction, not by an `if:` guard that someone could delete.
The path that makes them live is the export itself: `candace/` becomes the
repository root, and `candace/.github/` becomes `.github/`.

The monorepo has its own gates over the same content, and those are the ones a
change to this tree must satisfy before it can ever reach here:

| here | monorepo |
|---|---|
| `ci.yml` → `go`, `rust` | `.github/workflows/candace-bazel-checks.yml` |
| `ci.yml` → `identifiers` | `.github/workflows/component-export-checks.yml` |
| `ci.yml` → `candaceos` | `.github/workflows/candaceos-acceptance.yml` |
| `ci.yml` → `go_toolchain` | **no single counterpart.** The monorepo splits the same packages across `candace-go-checks.yml`, `gotth-live-checks.yml` and `pgmem-checks.yml`, and none of them runs `go test ./...` over the whole module. `//app/warden/e2e:e2e_test` is `manual` in Bazel and is reached by this job and nothing else, in either repository |
| `pages.yml` → `build` | `.github/workflows/blog-site-checks.yml` |

## What they are for

The destination is generated and read-only: the monorepo is canonical, and an
edit made there is overwritten by the next export. So these jobs are not a
contribution gate. They answer a different question — *is this snapshot
coherent on its own?* A snapshot can be green in the monorepo and still be
broken here, because here it is a repository rather than a subdirectory: the
module root moves, `candaceos/` sits at the top level, and consumers take this
tree as a Bazel module. `pages.yml` additionally does real work, publishing
blog.candace.cloud from `blog-site/`.

Three inert workflow copies that predate this directory were folded into it and
deleted: `blog-site/.github/workflows/pages.yml` (now `pages.yml` here, adapted
for a generator that is no longer the repository root), `pkg/pgmem/.github/
workflows/ci.yml`, and `xetcas/.github/workflows/ci.yaml`. Each targeted a
standalone repository that this monorepo export retires, and each was already
inert in both repositories — nested `.github/` directories are read by nobody.

## Conventions

- **GitHub-hosted `ubuntu-24.04` runners.** The monorepo's workflows target a
  self-hosted fleet that does not exist here.
- **Least privilege.** The workflow default is `contents: read`. Exactly one
  job — `pages.yml`'s `deploy` — asks for more, and only `pages: write` and
  `id-token: write`.
- **`actions/checkout` with `persist-credentials: false`.** No job in this
  repository writes to it.
- **Bazel comes from the pinned container**, through `tools/bazel.sh`, rather
  than from a runner-provided Bazel or a `setup-` action. It is the same
  command a developer runs, and `.bazelversion` and `MODULE.bazel` remain the
  only version authority. Bazel's caches are deliberately not carried between
  runs; the reasoning is in `ci.yml`.

## Operator prerequisites

`pages.yml` needs the destination repository's **Settings → Pages → Source** set
to *GitHub Actions*, and the custom domain `blog.candace.cloud` configured there
with DNS pointed at GitHub Pages. Until that is done the `deploy` job fails
while `build` still passes, which is the intended shape: the render and the
privacy scan are useful on their own.
