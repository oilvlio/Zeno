#!/usr/bin/env bash
# Guard against re-growing the oversized functions this codebase was refactored
# away from. Cyclomatic complexity is a proxy for how many independent paths a
# reviewer must hold in their head; past roughly 25 the admin update and agent
# ingest paths became hard to review, which is how partial-update bugs hid.
#
# The threshold is a ratchet, not an aspiration: lower it when the remaining
# offenders are fixed, never raise it to make a new function pass.
set -euo pipefail

GOCYCLO_VERSION="v0.6.0"
THRESHOLD="${COMPLEXITY_THRESHOLD:-25}"

cd "$(dirname "$0")/.."

# Known offenders predating the guard. Each entry must be removed, never added
# to, as these functions are split up.
ALLOWED_OVER_THRESHOLD=(
  "(*handler).handleAgentState"
  "(*handler).handleAgentProbeResults"
  "(*SQLiteStore).migrateProbeRoundIdempotency"
  "insertAgentStateSampleTx"
  "(*SQLiteStore).insertAgentProbeResultsOnce"
  "(*handler).handleAgentPresenceWebSocket"
)

# gocyclo exits non-zero whenever it reports a function, so `go run` prints an
# "exit status 1" line to stderr. The report itself is the signal, so stderr is
# discarded and the exit status ignored.
report="$(go run "github.com/fzipp/gocyclo/cmd/gocyclo@${GOCYCLO_VERSION}" \
  -over "${THRESHOLD}" -ignore '_test\.go' ./internal ./cmd 2>/dev/null || true)"

if [ -z "${report}" ]; then
  echo "complexity check: no function exceeds ${THRESHOLD}"
  exit 0
fi

violations=0
while IFS= read -r line; do
  [ -n "${line}" ] || continue
  # gocyclo output: "<complexity> <package> <function> <file>:<line>:<col>"
  complexity="$(printf '%s' "${line}" | awk '{print $1}')"
  function_name="$(printf '%s' "${line}" | awk '{print $3}')"
  allowed=0
  for entry in "${ALLOWED_OVER_THRESHOLD[@]}"; do
    if [ "${function_name}" = "${entry}" ]; then
      allowed=1
      break
    fi
  done
  if [ "${allowed}" -eq 1 ]; then
    echo "complexity check: known offender ${function_name} (${complexity})"
    continue
  fi
  echo "complexity check: FAIL ${line}" >&2
  violations=$((violations + 1))
done <<EOF
${report}
EOF

if [ "${violations}" -gt 0 ]; then
  echo "" >&2
  echo "${violations} function(s) exceed complexity ${THRESHOLD} and are not in the allowlist." >&2
  echo "Split the function instead of extending ALLOWED_OVER_THRESHOLD." >&2
  exit 1
fi

echo "complexity check: only known offenders exceed ${THRESHOLD}"
