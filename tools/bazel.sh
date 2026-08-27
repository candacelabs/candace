#!/usr/bin/env bash
# Run Bazel against this module inside the pinned Bazel container.
#
# There is no host Bazel and no host Go anywhere in this project's toolchain
# story: the Bazel release is pinned by digest here and by version in
# .bazelversion, and the Go SDK is downloaded by rules_go (see MODULE.bazel).
# That makes `tools/bazel.sh build //...` mean the same thing on a developer's
# machine and on a CI runner.
#
# Bazel's output base lives outside the checkout so a build never leaves
# anything in the worktree except the bazel-* convenience symlinks, which
# .gitignore covers. Set CANDACE_BAZEL_CACHE to move it.
#
# Usage: tools/bazel.sh <bazel arguments...>
set -Eeuo pipefail

bazel_image='gcr.io/bazel-public/bazel:9.2.0@sha256:e59bd66f8daf69f02dbfc18dbd72f0ecfe7926bbda95a5c9eb62433d83b8bd02'

die() {
  printf 'candace bazel: %s\n' "$*" >&2
  exit 1
}

command -v docker >/dev/null 2>&1 || die 'docker is required to run the pinned Bazel image'
[[ $# -gt 0 ]] || die 'no Bazel arguments were given'

module_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
cache_root=${CANDACE_BAZEL_CACHE:-${TMPDIR:-/tmp}/candace-bazel-cache}
mkdir -p -- "$cache_root/home" "$cache_root/output"

exec docker run --rm \
  --user "$(id -u):$(id -g)" \
  --env HOME=/bazel-home \
  --env USER="${USER:-bazel}" \
  --volume "$cache_root/home:/bazel-home" \
  --volume "$cache_root/output:/bazel-output" \
  --volume "$module_root:/candace" \
  --workdir /candace \
  --entrypoint /usr/local/bin/bazel \
  "$bazel_image" \
  --output_user_root=/bazel-output "$@"
