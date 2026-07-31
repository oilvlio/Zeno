#!/usr/bin/env bash
# Guard against re-growing the oversized functions this codebase was refactored
# away from. Cyclomatic complexity is a proxy for how many independent paths a
# reviewer must hold in their head; past roughly 25 the admin update and agent
# ingest paths became hard to review, which is how partial-update bugs hid.
#
# This started as a ratchet with an allowlist of pre-existing offenders. The
# allowlist is now empty: every function is under the threshold, so any new
# violation is a regression introduced by the change under review. Split the
# function rather than reintroducing an exception.
#
# The threshold may be lowered, never raised to make a new function pass.
set -euo pipefail

GOCYCLO_VERSION="v0.6.0"
THRESHOLD="${COMPLEXITY_THRESHOLD:-25}"

cd "$(dirname "$0")/.."

# gocyclo exits non-zero whenever it reports a function, so `go run` prints an
# "exit status 1" line to stderr. The report itself is the signal, so stderr is
# discarded and the exit status ignored.
report="$(go run "github.com/fzipp/gocyclo/cmd/gocyclo@${GOCYCLO_VERSION}" \
  -over "${THRESHOLD}" -ignore '_test\.go' ./internal ./cmd 2>/dev/null || true)"

if [ -z "${report}" ]; then
  echo "complexity check: no function exceeds ${THRESHOLD}"
  exit 0
fi

echo "complexity check: FAIL -- these functions exceed ${THRESHOLD}:" >&2
printf '%s\n' "${report}" >&2
echo "" >&2
echo "Split the function. The allowlist was emptied deliberately; do not add one back." >&2
exit 1
