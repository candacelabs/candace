#!/usr/bin/env bash
# Keep behavior tests in the public module's pkg/ and tools/ trees on the
# repository's Ginkgo/Gomega convention: a standard-library TestXxx function is
# allowed only as the RunSpecs bootstrap of a Ginkgo suite.
#
# tools/ joined the walk on 2026-09-02. The ifacereturn analyzer had shipped
# days earlier with a stock analysistest suite — two plain TestXxx functions —
# under a header comment that cited this script's own boundary as its
# justification: "check-test-style.sh scopes the Ginkgo convention to
# candace/pkg, so this package is outside it by that script's own boundary
# rather than by an exemption written here." That sentence was true, and being
# true is what made it expensive. Operator, on finding it: "IFACE RETURN TEST IS
# NOT USING GINKGOGOMEGAGOMOCK HOLY FUCK".
#
# The lesson is about scope rather than about that one suite. A gate that walks
# a subtree does not express a convention; it draws a line, and an author who
# reads the line correctly builds on the far side of it in good faith. So the
# walk grows with the module rather than the convention shrinking to fit the
# walk: pkg/ and tools/ are both first-party handwritten Go owned by this
# module, and both are now covered by the same rules with the same prune idiom.
#
# pkg/gotth is excluded. It arrived as a self-contained library with its own
# gate (pkg/gotth/ci.sh) and its own test conventions; folding it into this
# convention is a deliberate content change, not a side effect of a move.
set -euo pipefail

module_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
walk_roots=("${module_root}/pkg" "${module_root}/tools")
violations=()

# `find` over a path that does not exist reports the error and carries on, and
# its exit status is discarded by the process substitution below — so a root
# renamed out from under this script would silently narrow the walk to whatever
# is left and still print nothing. A gate that cannot read its corpus must never
# be able to render itself as a clean one.
for walk_root in "${walk_roots[@]}"; do
  if [[ ! -d "${walk_root}" ]]; then
    printf 'check-test-style: %s is not a directory; refusing a vacuous pass\n' \
      "${walk_root}" >&2
    exit 2
  fi
done

examined=0
while IFS= read -r -d '' test_file; do
  examined=$((examined + 1))
  test_count="$(grep -Ec '^func Test[[:alnum:]_]*\(t \*testing[.]T\)' "${test_file}" || true)"
  bootstrap_count="$(grep -Ec '^[[:space:]]*(ginkgo[.])?RunSpecs\(t,' "${test_file}" || true)"
  if ((test_count != bootstrap_count)); then
    violations+=(
      "${test_file#"${module_root}/"} (${test_count} Test functions, ${bootstrap_count} RunSpecs bootstraps)"
    )
  fi
done < <(find "${walk_roots[@]}" -path "${module_root}/pkg/gotth" -prune -o -type f -name '*_test.go' -print0)

if ((examined == 0)); then
  printf 'check-test-style: no *_test.go under %s; refusing a vacuous pass\n' \
    "${walk_roots[*]}" >&2
  exit 2
fi

if ((${#violations[@]} > 0)); then
  printf 'Standard-library behavior tests are not allowed; use Ginkgo/Gomega:\n' >&2
  printf '  %s\n' "${violations[@]}" >&2
  exit 1
fi
