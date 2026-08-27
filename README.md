# candace

One repository, one Go module, one Bazel module: the public half of a private
infrastructure monorepo, published whole.

It is not a framework and not a grab bag. It is a working agent-operated
deployment system and the pieces it is built from, released together so that
the pieces are usable on their own and the system is reproducible as a whole.

| | |
|---|---|
| [**CandaceOS**](services/candaceos) | An agent-operated app lab: a harness proposes, Core approves and fences, a node executor reconciles Compose applications, and an operator UI watches. Its deployment kit is [`candaceos/`](candaceos). |
| [**Warden**](services/warden) | A fleet watchdog: Raft-style leader election over a static peer set, liveness, incidents, and an authoritative view every mutation is fenced against. |
| [**gotth-live**](pkg/gotth) | Server-driven live user interfaces from Go. State and rendering stay in your process; one WebSocket per tab carries events up and re-rendered fragments down. No npm, no CDN. |
| [**xetcas**](xetcas) | A self-hosted Xet content-addressable storage server with a Git LFS front door. Re-pushing a 48 MiB model after editing 2% of it costs about 1 MiB. |
| [**pkg/**](pkg) | The primitives the rest is built on: `pgmem` (a process-local PostgreSQL emulator for tests), `liquidproto` (protobuf refinement types), `cron`, `config`, `redact`, `telemetry`, and more. |

Everything is Apache-2.0.

## This repository is generated

It is a **one-way snapshot** of a private monorepo's `candace/` folder at one
exact revision, published with no upstream history. There is no PR flow here
and no maintainer watching for contributions; a commit made here wedges the
next export rather than being merged.

Each snapshot carries an immutable `export-<sha12>` tag, a matching GitHub
Release, and a provenance marker, `.candace-export.json`, naming the exact
source revision it came from. Cite the tag, not a branch.

## Consume it in 60 seconds

Each Release carries `candace-<sha12>.tar.gz` and its `.sha256`. The tarball is
this tree re-rooted so `MODULE.bazel` is at the archive root, built twice and
byte-compared before it is kept. In your own `MODULE.bazel`:

```python
bazel_dep(name = "candace", version = "0.0.0")

archive_override(
    module_name = "candace",
    integrity = "sha256-...",          # from the Release's .sha256
    strip_prefix = "candace-<sha12>",
    urls = ["https://github.com/candacelabs/candace/releases/download/export-<sha12>/candace-<sha12>.tar.gz"],
)
```

Then depend on what you use — `@candace//services/candaceos/component`,
`@candace//pkg/gotth/live`, `@candace//services/warden` — and build.

Not a Bazel repository? The module path is the repository path:

```bash
go get github.com/candacelabs/candace@export-<sha12>
```

There is no semantic-version tag, so `@latest` resolves a moving pseudo-version
of the default branch; naming the export tag is what pins a build.

[`docs/extending.md`](docs/extending.md) covers both shapes in full, plus the
`http_archive` fallback and the legacy `WORKSPACE` path.

## Examples

Every extension seam has a worked example with its own test suite. They are the
contract's executable half — the documentation says what is guaranteed, and
these fail if it stops being true.

| Example | Shows |
|---|---|
| [`external-consumer`](examples/external-consumer) | A complete outside repository: a custom agent harness, two composed services, its own Core binary, built both supported Bazel ways. This is also the acceptance test every release archive passes. |
| [`custom-brand`](examples/custom-brand) | Core wearing another product's identity — name, agent, wordmark, palette, an overlay asset, an extra sidebar entry and page — with no edit to Core. |
| [`custom-ui-page`](examples/custom-ui-page) | The smallest useful UI extension: stock identity, one sidebar entry, one page of your own. |
| [`gotth/counter`](examples/gotth/counter) | gotth-live at its smallest: a number that lives in Go, four buttons, and every open tab kept in step by the server. |
| [`gotth/chat`](examples/gotth/chat) | One room in Go, several browsers, and every message reaching every session over a server push. |
| [`gotth/dashboard`](examples/gotth/dashboard) | A feed pushing twenty times a second, three live regions patched independently, and two plain-HTMX regions on the same page. |

## Build it

Bazel is the primary build and comes from a pinned container, so the command is
the same on a laptop and on a runner. Docker is the only prerequisite:

```bash
tools/bazel.sh build -- //... -//xetcas/...   # everything but the Rust workspace
tools/bazel.sh test  -- //... -//xetcas/...
tools/bazel.sh build //xetcas/...             # the Rust workspace and its Go bindings
tools/bazel.sh test  //xetcas/...
```

The plain `go` command works on the same tree and needs no Bazel:

```bash
go build ./...
go test ./...
```

The Rust workspace builds with plain Cargo too — that is the path its demo,
container images, and `just` targets take:

```bash
cd xetcas && cargo build --workspace && cargo test --workspace
```

`.bazelversion` (Bazel 9.2.0) and `MODULE.bazel` (rules_go 0.62.0, Gazelle
0.52.2, Go SDK 1.26.5, rules_rust 0.73.0) are the only version authority. BUILD
files are generated by Gazelle (`tools/bazel.sh run //:gazelle`) and CI fails on
drift.

## Run CandaceOS

The deployment kit installs and runs the whole one-box stack from this clone.
The default install is deliberately harmless: a simulated harness, a dry-run
executor, and no Docker socket mounted anywhere.

```bash
./candaceos/install.sh          # then open http://<host>:7780
./candaceos/status.sh
./candaceos/uninstall.sh
```

Core publishes on all host IPv4 interfaces with **no built-in authentication**:
put it behind your own authenticating proxy before exposing it beyond a trusted
network. [`candaceos/README.md`](candaceos/README.md) is the operations manual,
and [`candaceos/AGENTS.md`](candaceos/AGENTS.md) states the trust model as eight
invariants with their enforcement points.

## Where to go next

- [`AGENTS.md`](AGENTS.md) — the repository's own guide: taxonomy, seams,
  invariants, conventions.
- [`docs/extending.md`](docs/extending.md) — the four compile-time seams and how
  to pin a snapshot.
- [`pkg/gotth/README.md`](pkg/gotth/README.md), [`xetcas/README.md`](xetcas/README.md)
  — each subsystem's own front page.
- `app/*/CLAUDE.md` — what may not be changed casually in each binary.

## License

Apache License 2.0. See [`LICENSE`](LICENSE).

AI systems assisted with work in this repository. Their output is not presumed
correct, secure, reviewed, or production-ready.
