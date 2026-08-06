#!/usr/bin/env bash
set -euo pipefail

repo="${1:-${GITHUB_REPOSITORY:-}}"
sha="${2:-${GITHUB_SHA:-}}"
expected_branch="${3:-main}"
workflow="${CI_WORKFLOW:-CI}"
timeout_seconds="${CI_WAIT_TIMEOUT_SECONDS:-1200}"
interval_seconds="${CI_WAIT_INTERVAL_SECONDS:-10}"

[[ -n "$repo" ]] || { echo "repository is required" >&2; exit 1; }
[[ "$sha" =~ ^[0-9a-fA-F]{40}$ ]] || { echo "a full commit SHA is required" >&2; exit 1; }
[[ -n "$expected_branch" ]] || { echo "expected CI branch is required" >&2; exit 1; }
[[ "$timeout_seconds" =~ ^[1-9][0-9]*$ ]] || { echo "CI wait timeout must be a positive integer" >&2; exit 1; }
[[ "$interval_seconds" =~ ^[1-9][0-9]*$ ]] || { echo "CI wait interval must be a positive integer" >&2; exit 1; }
command -v gh >/dev/null || { echo "gh is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }

started_at=$SECONDS
while (( SECONDS - started_at < timeout_seconds )); do
  runs=$(gh run list \
    --repo "$repo" \
    --workflow "$workflow" \
    --commit "$sha" \
    --event push \
    --limit 50 \
    --json databaseId,workflowName,headBranch,headSha,status,conclusion,createdAt,url)
  selected=$(jq -c --arg branch "$expected_branch" --arg sha "${sha,,}" '
    [
      .[]
      | select(.headBranch == $branch)
      | select((.headSha | ascii_downcase) == $sha)
    ]
    | sort_by(.createdAt, .databaseId)
    | last // empty
  ' <<<"$runs")

  if [[ -n "$selected" ]]; then
    status=$(jq -r '.status' <<<"$selected")
    conclusion=$(jq -r '.conclusion // ""' <<<"$selected")
    url=$(jq -r '.url' <<<"$selected")
    if [[ "$status" == "completed" ]]; then
      if [[ "$conclusion" == "success" ]]; then
        echo "verified successful $workflow run for $sha on $expected_branch: $url"
        exit 0
      fi
      echo "$workflow run for $sha on $expected_branch concluded $conclusion: $url" >&2
      exit 1
    fi
    echo "waiting for $workflow run for $sha on $expected_branch (status=$status): $url"
  else
    echo "waiting for $workflow run to appear for $sha on $expected_branch"
  fi
  sleep "$interval_seconds"
done

echo "timed out waiting for successful $workflow run for $sha on $expected_branch" >&2
exit 1
