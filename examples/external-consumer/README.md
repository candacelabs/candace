# Building against candace from another repository

This is a complete Bazel repository that depends on `candace` the way a real
consumer does: it pins one published source archive, implements the CandaceOS
harness interface in its own Go package, composes two of its own services as
ordered components, links its own Core binary, and runs its own tests against
the module's public API. It never forks or vendors the source tree.

Everything under [`_workspace/`](_workspace) is that repository. The leading
underscore keeps it out of both toolchains that would otherwise claim it: the
`go` command ignores such a directory, and `.bazelignore` stops Bazel from
reading its `BUILD.bazel` files as packages of this module. They are not — they
belong to a build that resolves `@candace` to a downloaded archive.

## What it builds

- `steering/` — a bounded store and a service, composed with
  `component.WithRequires` so Core assembles and starts the store first and
  stops it last. Core constructs neither and reads neither one's configuration;
  it owns only the order.
- `customharness/` — a full `harness.Factory` and `harness.Runtime`
  implementation compiled outside the CandaceOS tree, publishing typed events
  through the host boundary and holding the steering service the composition
  root handed it.
- `cmd/` — the composition root: `bootstrap.Run` with two `WithComponent`
  options and one `WithHarnessFactory`, producing a Core binary with a
  different agent runtime and the stock control plane.
- The root `go_test` — Ginkgo specs over the harness and the component order,
  with a `gomock` host. They need no Core, no PostgreSQL, and no network.

## The two ways to pin the archive

Each release of `candacelabs/candace` carries a `candace-<sha12>.tar.gz` and its
`.sha256`. `MODULE.bazel` at the archive root makes the tarball a Bazel module,
so there are two shapes, and this example is built both ways before any archive
is published.

**`bazel_dep` + `archive_override` — use this one.**
[`_workspace/MODULE.archive-override.bazel.in`](_workspace/MODULE.archive-override.bazel.in)
names the module, pins the exact bytes with Subresource Integrity, and lets
candace's own `MODULE.bazel` supply its dependency closure. The consumer
declares four repositories: the ones its own targets use.

```python
bazel_dep(name = "candace", version = "0.0.0")

archive_override(
    module_name = "candace",
    integrity = "sha256-...",          # `integrity` from the packager
    strip_prefix = "candace-<sha12>",  # `strip_prefix` from the packager
    urls = ["https://github.com/candacelabs/candace/releases/download/export-<sha12>/candace-<sha12>.tar.gz"],
)
```

**`http_archive` — the fallback.**
[`_workspace/MODULE.http-archive.bazel.in`](_workspace/MODULE.http-archive.bazel.in)
fetches the same tarball without treating it as a module. That is sometimes what
a consumer wants, and it costs one thing: a repository fetched this way is not a
module, so the labels inside candace's BUILD files resolve through the
*consumer's* repository mapping. Every repository those files name has to be
visible in the consumer's `MODULE.bazel`, which means pasting candace's own
`use_repo(go_deps, ...)` block in and refreshing it when the archive moves. This
shape also has to download the Go SDK itself, because candace registers no
toolchain when it is not a module.

Both files carry `@CANDACE_ARCHIVE_...@` placeholders. The monorepo's
`tools/test_candace_external_consumer.sh` substitutes them with the lock
material `tools/package_candace_archive.sh` prints, serves the archive from a
throwaway container, and builds both consumers in the pinned Bazel image — which
is how a release learns that its archive works before anyone depends on it.

## Running it yourself

Against a published release, substitute the placeholders by hand from the
release's checksum and prefix, then, from inside a copy of `_workspace/`:

```bash
bazel build //cmd:custom-candaceos
bazel test //:external_harness_test
```

The resulting binary is a Linux Core executable with a custom harness compiled
in. `candaceos/README.md` describes how a fleet deployment layers such a binary
over the standard Core runtime.

## Consuming from a legacy WORKSPACE build

There is a second-class path for a repository that has not migrated to bzlmod.
It is documented in [`bazel/README.md`](../../bazel/README.md), it is not what
this example demonstrates, and no test in this repository can exercise it: the
Bazel release this module pins removed WORKSPACE support entirely.
