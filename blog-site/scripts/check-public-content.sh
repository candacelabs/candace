#!/usr/bin/env bash
# Privacy boundary check for the blog's rendered public output, ported from
# the standalone Hugo repository. CI mirrors the running blog container into
# a directory and scans every generated HTML/XML page for data that must
# never publish: contact details, private-profile links, and private or
# tailnet network addresses.

set -euo pipefail

SCAN_ROOT="${1:?usage: check-public-content.sh <rendered-output-dir>}"

if [[ ! -d "${SCAN_ROOT}" ]]; then
  echo "Public output not found: ${SCAN_ROOT}" >&2
  exit 2
fi

if ! command -v rg >/dev/null 2>&1; then
  echo "Privacy check cannot run: ripgrep (rg) is required." >&2
  exit 2
fi

RG_ERROR_LOG="$(mktemp)"
trap 'rm -f -- "${RG_ERROR_LOG}"' EXIT

RG_STATUS=0
PUBLIC_FILES="$(rg --files --glob '*.html' --glob '*.xml' "${SCAN_ROOT}" 2>"${RG_ERROR_LOG}")" || RG_STATUS=$?

if [[ "${RG_STATUS}" -gt 1 || -s "${RG_ERROR_LOG}" ]]; then
  echo "Privacy check could not inspect generated output: rg reported an error (status ${RG_STATUS})." >&2
  exit 2
fi

if [[ -z "${PUBLIC_FILES}" ]]; then
  echo "Privacy check cannot run: no generated HTML or XML files found in ${SCAN_ROOT}." >&2
  exit 2
fi

declare -a PATTERNS=(
  'mailto:'
  'linkedin\.com/in/'
  'docs\.google\.com/(document|spreadsheets|presentation)/d/'
  '\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b'
  '\b10\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\b'
  '\b192\.168\.[0-9]{1,3}\.[0-9]{1,3}\b'
  '\b172\.(1[6-9]|2[0-9]|3[01])\.[0-9]{1,3}\.[0-9]{1,3}\b'
  '\b100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.[0-9]{1,3}\.[0-9]{1,3}\b'
  'physical location'
  'hosted by:'
  'undergrad:'
)

FAILED=0

for PATTERN in "${PATTERNS[@]}"; do
  : > "${RG_ERROR_LOG}"
  RG_STATUS=0
  MATCHED_FILES="$(rg --ignore-case --files-with-matches --glob '*.html' --glob '*.xml' "${PATTERN}" "${SCAN_ROOT}" 2>"${RG_ERROR_LOG}")" || RG_STATUS=$?

  if [[ "${RG_STATUS}" -gt 1 || -s "${RG_ERROR_LOG}" ]]; then
    echo "Privacy check could not scan generated output: rg reported an error (status ${RG_STATUS})." >&2
    exit 2
  fi

  if [[ "${RG_STATUS}" -eq 0 ]]; then
    echo "Privacy check found a prohibited public-data pattern in:"
    while IFS= read -r MATCHED_FILE; do
      echo "  - ${MATCHED_FILE}"
    done <<< "${MATCHED_FILES}"
    FAILED=1
  fi
done

if [[ "${FAILED}" -ne 0 ]]; then
  echo "Privacy check failed: review the matches above." >&2
  exit 1
fi

echo "Privacy check passed."
