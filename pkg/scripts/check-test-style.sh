#!/usr/bin/env bash
# Keep behavior tests in the public module's pkg/ tree on the repository's
# Ginkgo/Gomega convention: a standard-library TestXxx function is allowed only
# as the RunSpecs bootstrap of a Ginkgo suite.
#
# pkg/gotth is excluded. It arrived as a self-contained library with its own
# gate (pkg/gotth/ci.sh) and its own test conventions; folding it into this
# convention is a deliberate content change, not a side effect of a move.
set -euo pipefail

pkg_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
violations=()

while IFS= read -r -d '' test_file; do
  test_count="$(grep -Ec '^func Test[[:alnum:]_]*\(t \*testing[.]T\)' "${test_file}" || true)"
  bootstrap_count="$(grep -Ec '^[[:space:]]*(ginkgo[.])?RunSpecs\(t,' "${test_file}" || true)"
  if ((test_count != bootstrap_count)); then
    violations+=(
      "${test_file#"${pkg_root}/"} (${test_count} Test functions, ${bootstrap_count} RunSpecs bootstraps)"
    )
  fi
done < <(find "${pkg_root}" -path "${pkg_root}/gotth" -prune -o -type f -name '*_test.go' -print0)

if ((${#violations[@]} > 0)); then
  printf 'Standard-library behavior tests are not allowed; use Ginkgo/Gomega:\n' >&2
  printf '  %s\n' "${violations[@]}" >&2
  exit 1
fi
